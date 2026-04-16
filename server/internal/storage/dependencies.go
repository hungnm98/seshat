package storage

import (
	"sort"

	"github.com/hungnm98/seshat-server/pkg/model"
)

func BuildFileDependencyGraph(version model.ProjectVersion, files []model.File, symbols []model.Symbol, relations []model.Relation, filePath string, depth int) (model.FileDependencyGraph, bool) {
	if depth <= 0 {
		depth = 1
	}

	fileByID := make(map[string]model.File, len(files))
	var rootFile model.File
	found := false
	for _, file := range files {
		fileByID[file.ID] = file
		if file.Path == filePath || file.ID == filePath {
			rootFile = file
			found = true
		}
	}
	if !found {
		return model.FileDependencyGraph{Version: &version}, false
	}

	symbolByID := make(map[string]model.Symbol, len(symbols))
	symbolsByFile := make(map[string][]model.Symbol)
	for _, symbol := range symbols {
		symbolByID[symbol.ID] = symbol
		symbolsByFile[symbol.FileID] = append(symbolsByFile[symbol.FileID], symbol)
	}
	rootSymbols := append([]model.Symbol(nil), symbolsByFile[rootFile.ID]...)
	sortSymbols(rootSymbols)

	graph := model.FileDependencyGraph{
		File:    rootFile,
		Symbols: rootSymbols,
		Version: &version,
	}
	if len(rootSymbols) == 0 {
		return graph, true
	}

	allowed := map[model.RelationType]struct{}{
		model.RelationCalls:      {},
		model.RelationReferences: {},
		model.RelationImports:    {},
		model.RelationImplements: {},
	}
	graph.DependsOn = collectFileDependencies(rootSymbols, symbolByID, fileByID, relations, allowed, depth, "outbound", rootFile.ID)
	graph.Dependents = collectFileDependencies(rootSymbols, symbolByID, fileByID, relations, allowed, depth, "inbound", rootFile.ID)

	relationSeen := make(map[string]struct{})
	for _, dep := range graph.DependsOn {
		for _, relation := range dep.Relations {
			if _, ok := relationSeen[relation.ID]; ok {
				continue
			}
			relationSeen[relation.ID] = struct{}{}
			graph.Relations = append(graph.Relations, relation)
		}
	}
	for _, dep := range graph.Dependents {
		for _, relation := range dep.Relations {
			if _, ok := relationSeen[relation.ID]; ok {
				continue
			}
			relationSeen[relation.ID] = struct{}{}
			graph.Relations = append(graph.Relations, relation)
		}
	}
	sortRelations(graph.Relations)

	return graph, true
}

func collectFileDependencies(rootSymbols []model.Symbol, symbolByID map[string]model.Symbol, fileByID map[string]model.File, relations []model.Relation, allowed map[model.RelationType]struct{}, maxDepth int, direction, rootFileID string) []model.FileDependency {
	frontier := make([]string, 0, len(rootSymbols))
	seenSymbols := make(map[string]struct{}, len(rootSymbols))
	for _, symbol := range rootSymbols {
		frontier = append(frontier, symbol.ID)
		seenSymbols[symbol.ID] = struct{}{}
	}

	depsByFile := make(map[string]*model.FileDependency)
	relationSeen := make(map[string]struct{})

	for level := 1; level <= maxDepth && len(frontier) > 0; level++ {
		var next []string
		for _, current := range frontier {
			for _, relation := range relations {
				if _, ok := allowed[relation.Type]; !ok {
					continue
				}

				var candidateID string
				switch direction {
				case "inbound":
					if relation.ToSymbolID == current {
						candidateID = relation.FromSymbolID
					}
				default:
					if relation.FromSymbolID == current {
						candidateID = relation.ToSymbolID
					}
				}
				if candidateID == "" {
					continue
				}

				candidate, ok := symbolByID[candidateID]
				if !ok {
					continue
				}

				if candidate.FileID != rootFileID {
					file, ok := fileByID[candidate.FileID]
					if ok {
						dep := depsByFile[file.ID]
						if dep == nil {
							dep = &model.FileDependency{File: file, Depth: level}
							depsByFile[file.ID] = dep
						}
						if level < dep.Depth {
							dep.Depth = level
						}
						appendUniqueSymbol(&dep.Symbols, candidate)
						appendUniqueRelation(&dep.Relations, relation)
						appendUniqueReason(&dep.Reasons, relation.Type)
					}
				}

				if _, ok := relationSeen[relation.ID]; !ok {
					relationSeen[relation.ID] = struct{}{}
				}
				if _, ok := seenSymbols[candidateID]; ok {
					continue
				}
				seenSymbols[candidateID] = struct{}{}
				next = append(next, candidateID)
			}
		}
		frontier = next
	}

	deps := make([]model.FileDependency, 0, len(depsByFile))
	for _, dep := range depsByFile {
		sortSymbols(dep.Symbols)
		sortRelations(dep.Relations)
		sort.Slice(dep.Reasons, func(i, j int) bool { return dep.Reasons[i] < dep.Reasons[j] })
		deps = append(deps, *dep)
	}
	sort.Slice(deps, func(i, j int) bool {
		if deps[i].Depth != deps[j].Depth {
			return deps[i].Depth < deps[j].Depth
		}
		return deps[i].File.Path < deps[j].File.Path
	})
	return deps
}

func appendUniqueSymbol(symbols *[]model.Symbol, symbol model.Symbol) {
	for _, existing := range *symbols {
		if existing.ID == symbol.ID {
			return
		}
	}
	*symbols = append(*symbols, symbol)
}

func appendUniqueRelation(relations *[]model.Relation, relation model.Relation) {
	for _, existing := range *relations {
		if existing.ID == relation.ID {
			return
		}
	}
	*relations = append(*relations, relation)
}

func appendUniqueReason(reasons *[]model.RelationType, reason model.RelationType) {
	for _, existing := range *reasons {
		if existing == reason {
			return
		}
	}
	*reasons = append(*reasons, reason)
}

func sortSymbols(symbols []model.Symbol) {
	sort.Slice(symbols, func(i, j int) bool {
		if symbols[i].Path != symbols[j].Path {
			return symbols[i].Path < symbols[j].Path
		}
		if symbols[i].LineStart != symbols[j].LineStart {
			return symbols[i].LineStart < symbols[j].LineStart
		}
		return symbols[i].ID < symbols[j].ID
	})
}

func sortRelations(relations []model.Relation) {
	sort.Slice(relations, func(i, j int) bool { return relations[i].ID < relations[j].ID })
}
