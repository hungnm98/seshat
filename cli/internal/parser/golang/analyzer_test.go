package golang

import (
	"context"
	"testing"

	"github.com/hungnm98/seshat-cli/internal/parser"
)

func TestAnalyzerExtractsSymbolsAndCalls(t *testing.T) {
	analyzer := New()
	batch, err := analyzer.Analyze(context.Background(), parser.Input{
		ProjectID:     "payment-service",
		RepoPath:      "../../../testdata/go_sample",
		IncludePaths:  []string{"cmd", "internal"},
		ExcludePaths:  []string{"vendor"},
		CommitSHA:     "abc123",
		Branch:        "main",
		SchemaVersion: "v1",
		ScanMode:      "full",
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if len(batch.Symbols) < 4 {
		t.Fatalf("expected symbols to be extracted, got %d", len(batch.Symbols))
	}
	found := false
	for _, relation := range batch.Relations {
		if relation.Type == "calls" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected at least one call relation")
	}
}
