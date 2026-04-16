package localquery

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hungnm98/seshat-cli/pkg/model"
)

const (
	DefaultLimit = 50
	DefaultDepth = 1
	MaxDepth     = 3
)

type Service struct {
	projectID string
	batch     model.AnalysisBatch
	symbols   map[string]model.Symbol
	files     map[string]model.File
}

func New(projectID string, batch model.AnalysisBatch) (*Service, error) {
	if batch.Metadata.ProjectID == "" {
		return nil, fmt.Errorf("graph metadata.project_id is required")
	}
	if projectID != "" && batch.Metadata.ProjectID != projectID {
		return nil, fmt.Errorf("invalid project_id %q for local index %q", projectID, batch.Metadata.ProjectID)
	}
	s := &Service{
		projectID: batch.Metadata.ProjectID,
		batch:     batch,
		symbols:   make(map[string]model.Symbol, len(batch.Symbols)),
		files:     make(map[string]model.File, len(batch.Files)),
	}
	for _, symbol := range batch.Symbols {
		s.symbols[symbol.ID] = symbol
	}
	for _, file := range batch.Files {
		s.files[file.ID] = file
	}
	return s, nil
}

func (s *Service) FindSymbol(projectID, query, kind string, limit int) ([]model.Symbol, *model.ProjectVersion, error) {
	if err := s.validateProject(projectID); err != nil {
		return nil, nil, err
	}
	limit = normalizeLimit(limit)
	needle := strings.ToLower(strings.TrimSpace(query))
	kind = strings.ToLower(strings.TrimSpace(kind))
	results := make([]model.Symbol, 0)
	for _, symbol := range s.batch.Symbols {
		if kind != "" && strings.ToLower(symbol.Kind) != kind {
			continue
		}
		if needle != "" && !matchesSymbol(symbol, needle) {
			continue
		}
		results = append(results, symbol)
	}
	sortSymbols(results)
	if len(results) > limit {
		results = results[:limit]
	}
	return results, s.version(), nil
}

func (s *Service) GetSymbolDetail(projectID, symbolID string) (model.QuerySymbolResult, bool, error) {
	if err := s.validateProject(projectID); err != nil {
		return model.QuerySymbolResult{}, false, err
	}
	symbol, ok := s.symbols[symbolID]
	if !ok {
		return model.QuerySymbolResult{}, false, nil
	}
	inbound := make([]model.Relation, 0)
	outbound := make([]model.Relation, 0)
	for _, relation := range s.batch.Relations {
		if relation.ToSymbolID == symbolID {
			inbound = append(inbound, relation)
		}
		if relation.FromSymbolID == symbolID {
			outbound = append(outbound, relation)
		}
	}
	sortRelations(inbound)
	sortRelations(outbound)
	return model.QuerySymbolResult{Symbol: symbol, Inbound: inbound, Outbound: outbound, Version: s.version()}, true, nil
}

func (s *Service) FindCallers(projectID, symbolID string, depth int) ([]model.Symbol, []model.Relation, *model.ProjectVersion, error) {
	return s.traverseCalls(projectID, symbolID, depth, "callers")
}

func (s *Service) FindCallees(projectID, symbolID string, depth int) ([]model.Symbol, []model.Relation, *model.ProjectVersion, error) {
	return s.traverseCalls(projectID, symbolID, depth, "callees")
}

func (s *Service) FileDependencyGraph(projectID, filePath string, depth int) (model.FileDependencyGraph, bool, error) {
	if err := s.validateProject(projectID); err != nil {
		return model.FileDependencyGraph{}, false, err
	}
	depth = normalizeDepth(depth)
	var root model.File
	found := false
	for _, file := range s.batch.Files {
		if file.Path == filePath {
			root = file
			found = true
			break
		}
	}
	if !found {
		return model.FileDependencyGraph{}, false, nil
	}
	rootSymbols := s.symbolsForFile(root.ID)
	rootSet := symbolSet(rootSymbols)
	depends := make(map[string]*model.FileDependency)
	dependents := make(map[string]*model.FileDependency)
	relatedRelations := make([]model.Relation, 0)
	addDependency := func(target map[string]*model.FileDependency, otherFile model.File, relation *model.Relation, reason model.RelationType) {
		entry := target[otherFile.ID]
		if entry == nil {
			entry = &model.FileDependency{File: otherFile, Depth: depth}
			target[otherFile.ID] = entry
		}
		if relation != nil {
			entry.Relations = append(entry.Relations, *relation)
		}
		entry.Reasons = appendReason(entry.Reasons, reason)
	}
	for _, relation := range s.batch.Relations {
		fromRoot := rootSet[relation.FromSymbolID]
		toRoot := rootSet[relation.ToSymbolID]
		if !fromRoot && !toRoot {
			continue
		}
		otherID := relation.ToSymbolID
		target := depends
		if toRoot {
			otherID = relation.FromSymbolID
			target = dependents
		}
		otherSymbol, ok := s.symbols[otherID]
		if !ok || otherSymbol.FileID == root.ID {
			continue
		}
		otherFile, ok := s.files[otherSymbol.FileID]
		if !ok {
			continue
		}
		addDependency(target, otherFile, &relation, relation.Type)
		relatedRelations = append(relatedRelations, relation)
	}
	for _, symbol := range rootSymbols {
		if symbol.Kind != "import" || symbol.Signature == "" {
			continue
		}
		for _, file := range s.batch.Files {
			if file.ID == root.ID || file.Language != root.Language {
				continue
			}
			if strings.HasSuffix(file.Path, "_test.go") {
				continue
			}
			if importMatchesFile(symbol.Signature, file.Path) {
				addDependency(depends, file, nil, model.RelationImports)
			}
		}
	}
	return model.FileDependencyGraph{
		File:       root,
		Symbols:    rootSymbols,
		DependsOn:  dependencyList(depends, s),
		Dependents: dependencyList(dependents, s),
		Relations:  sortedRelationCopy(relatedRelations),
		Version:    s.version(),
	}, true, nil
}

func importMatchesFile(importPath, filePath string) bool {
	importPath = filepath.ToSlash(importPath)
	filePath = filepath.ToSlash(filePath)
	switch {
	case strings.Contains(importPath, "/seshat-cli/") && !strings.HasPrefix(filePath, "cli/"):
		return false
	case strings.Contains(importPath, "/seshat-server/") && !strings.HasPrefix(filePath, "server/"):
		return false
	}
	fileDir := filepath.ToSlash(filepath.Dir(filePath))
	if fileDir == "." || fileDir == "" {
		return false
	}
	return strings.HasSuffix(importPath, fileDir) || strings.HasSuffix(importPath, strings.TrimPrefix(fileDir, "cli/")) || strings.HasSuffix(importPath, strings.TrimPrefix(fileDir, "server/"))
}

func (s *Service) ProjectID() string {
	return s.projectID
}

func (s *Service) validateProject(projectID string) error {
	if strings.TrimSpace(projectID) == "" {
		return fmt.Errorf("project_id is required")
	}
	if projectID != s.projectID {
		return fmt.Errorf("invalid project_id %q for local index %q", projectID, s.projectID)
	}
	return nil
}

func (s *Service) version() *model.ProjectVersion {
	return &model.ProjectVersion{
		ID:         s.batch.Metadata.ProjectID + ":" + s.batch.Metadata.CommitSHA,
		ProjectID:  s.batch.Metadata.ProjectID,
		CommitSHA:  s.batch.Metadata.CommitSHA,
		Branch:     s.batch.Metadata.Branch,
		Status:     "indexed",
		Schema:     s.batch.Metadata.SchemaVersion,
		ScannedAt:  s.batch.Metadata.GeneratedAt,
		FilesCount: len(s.batch.Files),
		NodesCount: len(s.batch.Symbols),
		EdgesCount: len(s.batch.Relations),
	}
}

func (s *Service) traverseCalls(projectID, symbolID string, depth int, direction string) ([]model.Symbol, []model.Relation, *model.ProjectVersion, error) {
	if err := s.validateProject(projectID); err != nil {
		return nil, nil, nil, err
	}
	depth = normalizeDepth(depth)
	seenSymbols := make(map[string]struct{})
	seenRelations := make(map[string]struct{})
	frontier := []string{symbolID}
	for level := 0; level < depth && len(frontier) > 0; level++ {
		next := make([]string, 0)
		for _, current := range frontier {
			for _, relation := range s.batch.Relations {
				if relation.Type != model.RelationCalls {
					continue
				}
				target := ""
				switch direction {
				case "callers":
					if relation.ToSymbolID == current {
						target = relation.FromSymbolID
					}
				case "callees":
					if relation.FromSymbolID == current {
						target = relation.ToSymbolID
					}
				}
				if target == "" {
					continue
				}
				seenRelations[relation.ID] = struct{}{}
				if _, ok := seenSymbols[target]; ok {
					continue
				}
				seenSymbols[target] = struct{}{}
				next = append(next, target)
			}
		}
		frontier = next
	}
	symbols := make([]model.Symbol, 0, len(seenSymbols))
	for id := range seenSymbols {
		if symbol, ok := s.symbols[id]; ok {
			symbols = append(symbols, symbol)
		}
	}
	relations := make([]model.Relation, 0, len(seenRelations))
	for _, relation := range s.batch.Relations {
		if _, ok := seenRelations[relation.ID]; ok {
			relations = append(relations, relation)
		}
	}
	sortSymbols(symbols)
	sortRelations(relations)
	return symbols, relations, s.version(), nil
}

func (s *Service) symbolsForFile(fileID string) []model.Symbol {
	symbols := make([]model.Symbol, 0)
	for _, symbol := range s.batch.Symbols {
		if symbol.FileID == fileID {
			symbols = append(symbols, symbol)
		}
	}
	sortSymbols(symbols)
	return symbols
}

func normalizeLimit(limit int) int {
	if limit <= 0 || limit > DefaultLimit {
		return DefaultLimit
	}
	return limit
}

func normalizeDepth(depth int) int {
	if depth <= 0 {
		return DefaultDepth
	}
	if depth > MaxDepth {
		return MaxDepth
	}
	return depth
}

func matchesSymbol(symbol model.Symbol, needle string) bool {
	return strings.Contains(strings.ToLower(symbol.Name), needle) ||
		strings.Contains(strings.ToLower(symbol.Signature), needle) ||
		strings.Contains(strings.ToLower(symbol.ID), needle)
}

func sortSymbols(symbols []model.Symbol) {
	sort.Slice(symbols, func(i, j int) bool {
		if symbols[i].Path == symbols[j].Path {
			return symbols[i].ID < symbols[j].ID
		}
		return symbols[i].Path < symbols[j].Path
	})
}

func sortRelations(relations []model.Relation) {
	sort.Slice(relations, func(i, j int) bool { return relations[i].ID < relations[j].ID })
}

func sortedRelationCopy(relations []model.Relation) []model.Relation {
	out := append([]model.Relation(nil), relations...)
	sortRelations(out)
	return out
}

func symbolSet(symbols []model.Symbol) map[string]bool {
	set := make(map[string]bool, len(symbols))
	for _, symbol := range symbols {
		set[symbol.ID] = true
	}
	return set
}

func appendReason(reasons []model.RelationType, reason model.RelationType) []model.RelationType {
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}

func dependencyList(items map[string]*model.FileDependency, s *Service) []model.FileDependency {
	out := make([]model.FileDependency, 0, len(items))
	for _, item := range items {
		item.Symbols = s.symbolsForFile(item.File.ID)
		sortRelations(item.Relations)
		sort.Slice(item.Reasons, func(i, j int) bool { return item.Reasons[i] < item.Reasons[j] })
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].File.Path < out[j].File.Path })
	return out
}

func GeneratedAt(batch model.AnalysisBatch) time.Time {
	return batch.Metadata.GeneratedAt
}
