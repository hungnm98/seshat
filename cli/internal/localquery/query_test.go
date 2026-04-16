package localquery

import (
	"testing"
	"time"

	"github.com/hungnm98/seshat-cli/pkg/model"
)

func TestLocalQuerySymbolAndCalls(t *testing.T) {
	service := testService(t)

	results, _, err := service.FindSymbol("proj", "Create", "", 10)
	if err != nil {
		t.Fatalf("FindSymbol returned error: %v", err)
	}
	if len(results) != 1 || results[0].Name != "CreateOrder" {
		t.Fatalf("unexpected symbol results: %#v", results)
	}

	callees, relations, _, err := service.FindCallees("proj", "symbol:go:order:func:CreateOrder", 10)
	if err != nil {
		t.Fatalf("FindCallees returned error: %v", err)
	}
	if len(callees) != 1 || callees[0].Name != "Validate" || len(relations) != 1 {
		t.Fatalf("unexpected callees: %#v %#v", callees, relations)
	}

	callers, _, _, err := service.FindCallers("proj", "symbol:go:order:func:Validate", 1)
	if err != nil {
		t.Fatalf("FindCallers returned error: %v", err)
	}
	if len(callers) != 1 || callers[0].Name != "CreateOrder" {
		t.Fatalf("unexpected callers: %#v", callers)
	}
}

func TestLocalQueryFileDependencyGraph(t *testing.T) {
	service := testService(t)

	graph, ok, err := service.FileDependencyGraph("proj", "internal/order/service.go", 1)
	if err != nil {
		t.Fatalf("FileDependencyGraph returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected graph to be found")
	}
	if len(graph.DependsOn) != 1 || graph.DependsOn[0].File.Path != "internal/order/validate.go" {
		t.Fatalf("unexpected dependency graph: %#v", graph)
	}
}

func TestLocalQueryFileDependencyGraphMapsLocalImports(t *testing.T) {
	service, err := New("proj", model.AnalysisBatch{
		Metadata: model.GraphMetadata{ProjectID: "proj"},
		Files: []model.File{
			{ID: "file:go:cmd/app/main.go", Path: "cmd/app/main.go", Language: "go"},
			{ID: "file:go:internal/localindex/index.go", Path: "internal/localindex/index.go", Language: "go"},
		},
		Symbols: []model.Symbol{
			{ID: "symbol:go:import:main@cmd/app:localindex", FileID: "file:go:cmd/app/main.go", Kind: "import", Name: "localindex", Signature: "github.com/acme/project/internal/localindex", Path: "cmd/app/main.go"},
			{ID: "symbol:go:localindex@internal/localindex:package", FileID: "file:go:internal/localindex/index.go", Kind: "package", Name: "localindex", Path: "internal/localindex/index.go"},
		},
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	graph, ok, err := service.FileDependencyGraph("proj", "cmd/app/main.go", 1)
	if err != nil {
		t.Fatalf("FileDependencyGraph returned error: %v", err)
	}
	if !ok || len(graph.DependsOn) != 1 || graph.DependsOn[0].File.Path != "internal/localindex/index.go" {
		t.Fatalf("unexpected import dependency graph: %#v", graph)
	}
}

func TestLocalQueryRejectsWrongProject(t *testing.T) {
	service := testService(t)
	if _, _, err := service.FindSymbol("other", "Create", "", 10); err == nil {
		t.Fatal("expected wrong project error")
	}
}

func testService(t *testing.T) *Service {
	t.Helper()
	service, err := New("proj", model.AnalysisBatch{
		Metadata: model.GraphMetadata{
			ProjectID:     "proj",
			CommitSHA:     "abc",
			Branch:        "main",
			SchemaVersion: "v1",
			GeneratedAt:   time.Unix(10, 0).UTC(),
			ScanMode:      "full",
		},
		Files: []model.File{
			{ID: "file:go:internal/order/service.go", Path: "internal/order/service.go", Language: "go"},
			{ID: "file:go:internal/order/validate.go", Path: "internal/order/validate.go", Language: "go"},
		},
		Symbols: []model.Symbol{
			{ID: "symbol:go:order:func:CreateOrder", FileID: "file:go:internal/order/service.go", Kind: "function", Name: "CreateOrder", Path: "internal/order/service.go"},
			{ID: "symbol:go:order:func:Validate", FileID: "file:go:internal/order/validate.go", Kind: "function", Name: "Validate", Path: "internal/order/validate.go"},
		},
		Relations: []model.Relation{
			{ID: "relation:calls", ProjectID: "proj", FromSymbolID: "symbol:go:order:func:CreateOrder", ToSymbolID: "symbol:go:order:func:Validate", Type: model.RelationCalls},
		},
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return service
}
