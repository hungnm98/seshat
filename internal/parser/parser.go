package parser

import (
	"context"
	"fmt"
	"sort"

	"github.com/hungnm98/seshat/pkg/model"
)

type Input struct {
	ProjectID     string
	RepoPath      string
	IncludePaths  []string
	ExcludePaths  []string
	CommitSHA     string
	Branch        string
	SchemaVersion string
	ScanMode      string
}

type Analyzer interface {
	Language() string
	Analyze(ctx context.Context, input Input) (model.AnalysisBatch, error)
}

func MergeBatches(projectID string, commitSHA string, branch string, schemaVersion string, scanMode string, batches ...model.AnalysisBatch) model.AnalysisBatch {
	merged := model.AnalysisBatch{
		Metadata: model.GraphMetadata{
			ProjectID:     projectID,
			CommitSHA:     commitSHA,
			Branch:        branch,
			SchemaVersion: schemaVersion,
			GeneratedAt:   nowUTC(),
			ScanMode:      scanMode,
		},
	}

	fileIndex := make(map[string]model.File)
	symbolIndex := make(map[string]model.Symbol)
	relationIndex := make(map[string]model.Relation)

	for _, batch := range batches {
		for _, file := range batch.Files {
			fileIndex[file.ID] = file
		}
		for _, symbol := range batch.Symbols {
			symbolIndex[symbol.ID] = symbol
		}
		for _, relation := range batch.Relations {
			relationIndex[relation.ID] = relation
		}
	}

	for _, file := range fileIndex {
		merged.Files = append(merged.Files, file)
	}
	for _, symbol := range symbolIndex {
		merged.Symbols = append(merged.Symbols, symbol)
	}
	for _, relation := range relationIndex {
		merged.Relations = append(merged.Relations, relation)
	}

	sort.Slice(merged.Files, func(i, j int) bool { return merged.Files[i].Path < merged.Files[j].Path })
	sort.Slice(merged.Symbols, func(i, j int) bool { return merged.Symbols[i].ID < merged.Symbols[j].ID })
	sort.Slice(merged.Relations, func(i, j int) bool { return merged.Relations[i].ID < merged.Relations[j].ID })

	return merged
}

func ValidateTargets(targets []string, analyzers map[string]Analyzer) error {
	for _, target := range targets {
		if _, ok := analyzers[target]; !ok {
			return fmt.Errorf("unsupported language target %q", target)
		}
	}
	return nil
}
