package query

import (
	"context"
	"testing"
	"time"

	"github.com/hungnm98/seshat-server/internal/storage/memory"
	"github.com/hungnm98/seshat-server/pkg/model"
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

func TestFileDependencyGraph(t *testing.T) {
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
		Files: []model.File{
			{ID: "file:go:handler.go", Path: "handler.go", Language: "go", Checksum: "sum-1"},
			{ID: "file:go:service.go", Path: "service.go", Language: "go", Checksum: "sum-2"},
			{ID: "file:go:repo.go", Path: "repo.go", Language: "go", Checksum: "sum-3"},
			{ID: "file:go:audit.go", Path: "audit.go", Language: "go", Checksum: "sum-4"},
		},
		Symbols: []model.Symbol{
			{ID: "handler", FileID: "file:go:handler.go", Kind: "function", Name: "Handler", Language: "go", Path: "handler.go", LineStart: 1, LineEnd: 2},
			{ID: "service", FileID: "file:go:service.go", Kind: "function", Name: "Service", Language: "go", Path: "service.go", LineStart: 1, LineEnd: 2},
			{ID: "repo", FileID: "file:go:repo.go", Kind: "function", Name: "Repo", Language: "go", Path: "repo.go", LineStart: 1, LineEnd: 2},
			{ID: "audit", FileID: "file:go:audit.go", Kind: "function", Name: "Audit", Language: "go", Path: "audit.go", LineStart: 1, LineEnd: 2},
		},
		Relations: []model.Relation{
			{ID: "r1", ProjectID: "proj-a", FromSymbolID: "handler", ToSymbolID: "service", Type: model.RelationCalls},
			{ID: "r2", ProjectID: "proj-a", FromSymbolID: "service", ToSymbolID: "repo", Type: model.RelationCalls},
			{ID: "r3", ProjectID: "proj-a", FromSymbolID: "audit", ToSymbolID: "service", Type: model.RelationReferences},
		},
	}, []byte(`{}`))
	if err != nil {
		t.Fatalf("StoreBatch: %v", err)
	}

	service := NewService(store)
	graph, ok, err := service.FileDependencyGraph(context.Background(), "proj-a", "service.go", 1)
	if err != nil {
		t.Fatalf("FileDependencyGraph: %v", err)
	}
	if !ok {
		t.Fatalf("expected service.go to be found")
	}
	if graph.File.Path != "service.go" {
		t.Fatalf("unexpected root file: %#v", graph.File)
	}
	if len(graph.DependsOn) != 1 || graph.DependsOn[0].File.Path != "repo.go" {
		t.Fatalf("expected service.go to depend on repo.go, got %#v", graph.DependsOn)
	}
	if len(graph.Dependents) != 2 {
		t.Fatalf("expected handler.go and audit.go to depend on service.go, got %#v", graph.Dependents)
	}
	dependentPaths := []string{graph.Dependents[0].File.Path, graph.Dependents[1].File.Path}
	if dependentPaths[0] != "audit.go" || dependentPaths[1] != "handler.go" {
		t.Fatalf("unexpected dependent paths: %#v", dependentPaths)
	}
}
