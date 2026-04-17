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
	projectID       string
	batch           model.AnalysisBatch
	symbols         map[string]model.Symbol
	symbolsByFile   map[string][]model.Symbol
	symbolsByKind   map[string][]model.Symbol
	files           map[string]model.File
	filesByPath     map[string]model.File
	filesByLanguage map[string][]model.File
	relationsByID   map[string]model.Relation
	relationsByFrom map[model.RelationType]map[string][]model.Relation
	relationsByTo   map[model.RelationType]map[string][]model.Relation
}

func New(projectID string, batch model.AnalysisBatch) (*Service, error) {
	if batch.Metadata.ProjectID == "" {
		return nil, fmt.Errorf("graph metadata.project_id is required")
	}
	if projectID != "" && batch.Metadata.ProjectID != projectID {
		return nil, fmt.Errorf("invalid project_id %q for local index %q", projectID, batch.Metadata.ProjectID)
	}
	s := &Service{
		projectID:       batch.Metadata.ProjectID,
		batch:           batch,
		symbols:         make(map[string]model.Symbol, len(batch.Symbols)),
		symbolsByFile:   make(map[string][]model.Symbol),
		symbolsByKind:   make(map[string][]model.Symbol),
		files:           make(map[string]model.File, len(batch.Files)),
		filesByPath:     make(map[string]model.File, len(batch.Files)),
		filesByLanguage: make(map[string][]model.File),
		relationsByID:   make(map[string]model.Relation, len(batch.Relations)),
		relationsByFrom: make(map[model.RelationType]map[string][]model.Relation),
		relationsByTo:   make(map[model.RelationType]map[string][]model.Relation),
	}
	for _, symbol := range batch.Symbols {
		s.symbols[symbol.ID] = symbol
		s.symbolsByFile[symbol.FileID] = append(s.symbolsByFile[symbol.FileID], symbol)
		kind := strings.ToLower(symbol.Kind)
		s.symbolsByKind[kind] = append(s.symbolsByKind[kind], symbol)
	}
	for _, file := range batch.Files {
		s.files[file.ID] = file
		s.filesByPath[file.Path] = file
		s.filesByLanguage[file.Language] = append(s.filesByLanguage[file.Language], file)
	}
	for _, relation := range batch.Relations {
		s.relationsByID[relation.ID] = relation
		indexRelation(s.relationsByFrom, relation.Type, relation.FromSymbolID, relation)
		indexRelation(s.relationsByTo, relation.Type, relation.ToSymbolID, relation)
	}
	for fileID := range s.symbolsByFile {
		sortSymbols(s.symbolsByFile[fileID])
	}
	for kind := range s.symbolsByKind {
		sortSymbols(s.symbolsByKind[kind])
	}
	for language := range s.filesByLanguage {
		sort.Slice(s.filesByLanguage[language], func(i, j int) bool {
			return s.filesByLanguage[language][i].Path < s.filesByLanguage[language][j].Path
		})
	}
	for _, bySymbol := range s.relationsByFrom {
		for symbolID := range bySymbol {
			sortRelations(bySymbol[symbolID])
		}
	}
	for _, bySymbol := range s.relationsByTo {
		for symbolID := range bySymbol {
			sortRelations(bySymbol[symbolID])
		}
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
	candidates := s.batch.Symbols
	if kind != "" {
		candidates = s.symbolsByKind[kind]
	}
	results := make([]model.Symbol, 0)
	for _, symbol := range candidates {
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
	for _, bySymbol := range s.relationsByTo {
		inbound = append(inbound, bySymbol[symbolID]...)
	}
	for _, bySymbol := range s.relationsByFrom {
		outbound = append(outbound, bySymbol[symbolID]...)
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
	root, found := s.filesByPath[filePath]
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
	seenRelated := make(map[string]struct{})
	for _, symbol := range rootSymbols {
		for _, bySymbol := range s.relationsByFrom {
			for _, relation := range bySymbol[symbol.ID] {
				if _, ok := rootSet[relation.FromSymbolID]; !ok {
					continue
				}
				otherSymbol, ok := s.symbols[relation.ToSymbolID]
				if !ok || otherSymbol.FileID == root.ID {
					continue
				}
				otherFile, ok := s.files[otherSymbol.FileID]
				if !ok {
					continue
				}
				addDependency(depends, otherFile, &relation, relation.Type)
				if _, ok := seenRelated[relation.ID]; !ok {
					relatedRelations = append(relatedRelations, relation)
					seenRelated[relation.ID] = struct{}{}
				}
			}
		}
		for _, bySymbol := range s.relationsByTo {
			for _, relation := range bySymbol[symbol.ID] {
				if _, ok := rootSet[relation.ToSymbolID]; !ok {
					continue
				}
				otherSymbol, ok := s.symbols[relation.FromSymbolID]
				if !ok || otherSymbol.FileID == root.ID {
					continue
				}
				otherFile, ok := s.files[otherSymbol.FileID]
				if !ok {
					continue
				}
				addDependency(dependents, otherFile, &relation, relation.Type)
				if _, ok := seenRelated[relation.ID]; !ok {
					relatedRelations = append(relatedRelations, relation)
					seenRelated[relation.ID] = struct{}{}
				}
			}
		}
	}
	for _, symbol := range rootSymbols {
		if symbol.Kind != "import" || symbol.Signature == "" {
			continue
		}
		for _, file := range s.filesByLanguage[root.Language] {
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
			for _, relation := range s.callRelations(current, direction) {
				target := ""
				switch direction {
				case "callers":
					target = relation.FromSymbolID
				case "callees":
					target = relation.ToSymbolID
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
	for id := range seenRelations {
		if relation, ok := s.relationsByID[id]; ok {
			relations = append(relations, relation)
		}
	}
	sortSymbols(symbols)
	sortRelations(relations)
	return symbols, relations, s.version(), nil
}

func (s *Service) symbolsForFile(fileID string) []model.Symbol {
	return append([]model.Symbol(nil), s.symbolsByFile[fileID]...)
}

func (s *Service) callRelations(symbolID, direction string) []model.Relation {
	switch direction {
	case "callers":
		return s.relationsByTo[model.RelationCalls][symbolID]
	case "callees":
		return s.relationsByFrom[model.RelationCalls][symbolID]
	default:
		return nil
	}
}

func indexRelation(index map[model.RelationType]map[string][]model.Relation, relationType model.RelationType, symbolID string, relation model.Relation) {
	if index[relationType] == nil {
		index[relationType] = make(map[string][]model.Relation)
	}
	index[relationType][symbolID] = append(index[relationType][symbolID], relation)
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
