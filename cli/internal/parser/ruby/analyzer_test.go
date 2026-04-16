package ruby

import (
	"context"
	"testing"

	"github.com/hungnm98/seshat-cli/internal/parser"
)

func TestAnalyzerExtractsRubySymbols(t *testing.T) {
	analyzer := New()
	batch, err := analyzer.Analyze(context.Background(), parser.Input{
		ProjectID:     "rails-app",
		RepoPath:      "../../../testdata/ruby_sample",
		IncludePaths:  []string{"app"},
		ExcludePaths:  []string{"vendor"},
		CommitSHA:     "abc123",
		Branch:        "main",
		SchemaVersion: "v1",
		ScanMode:      "full",
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if len(batch.Symbols) == 0 {
		t.Fatalf("expected ruby symbols to be extracted")
	}
}
