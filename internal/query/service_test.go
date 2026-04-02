package query

import (
	"context"
	"testing"
	"time"

	"github.com/hungnm98/seshat/internal/storage/memory"
	"github.com/hungnm98/seshat/pkg/model"
)

func TestFindCallersAndCallees(t *testing.T) {
	store := memory.New()
	if _, err := store.CreateProject(context.Background(), model.Project{ID: "proj-a", Name: "A", DefaultBranch: "main"}); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	_, _, err := store.StoreBatch(context.Background(), model.AnalysisBatch{
		Metadata: model.GraphMetadata{
			ProjectID:     "proj-a",
			CommitSHA:     "abc123",
			Branch:        "main",
			SchemaVersion: "v1",
			GeneratedAt:   time.Now().UTC(),
		},
		Files: []model.File{{ID: "file:go:service.go", Path: "service.go", Language: "go", Checksum: "sum"}},
		Symbols: []model.Symbol{
			{ID: "a", FileID: "file:go:service.go", Kind: "function", Name: "A", Language: "go", Path: "service.go", LineStart: 1, LineEnd: 2},
			{ID: "b", FileID: "file:go:service.go", Kind: "function", Name: "B", Language: "go", Path: "service.go", LineStart: 3, LineEnd: 4},
			{ID: "c", FileID: "file:go:service.go", Kind: "function", Name: "C", Language: "go", Path: "service.go", LineStart: 5, LineEnd: 6},
		},
		Relations: []model.Relation{
			{ID: "r1", ProjectID: "proj-a", FromSymbolID: "a", ToSymbolID: "b", Type: model.RelationCalls},
			{ID: "r2", ProjectID: "proj-a", FromSymbolID: "b", ToSymbolID: "c", Type: model.RelationCalls},
		},
	}, []byte(`{}`))
	if err != nil {
		t.Fatalf("StoreBatch: %v", err)
	}
	service := NewService(store)
	callers, _, _, err := service.FindCallers(context.Background(), "proj-a", "c", 2)
	if err != nil {
		t.Fatalf("FindCallers: %v", err)
	}
	if len(callers) != 2 {
		t.Fatalf("expected 2 callers across depth, got %d", len(callers))
	}
	callees, _, _, err := service.FindCallees(context.Background(), "proj-a", "a", 2)
	if err != nil {
		t.Fatalf("FindCallees: %v", err)
	}
	if len(callees) != 2 {
		t.Fatalf("expected 2 callees across depth, got %d", len(callees))
	}
}
