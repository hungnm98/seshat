package graphrender

import (
	"strings"
	"testing"

	"github.com/hungnm98/seshat-cli/pkg/model"
)

func TestRenderMermaid(t *testing.T) {
	out, err := Render(testGraph(), Options{Format: FormatMermaid, Direction: DirectionBoth, MaxNodes: 10})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if !strings.Contains(out, "flowchart LR") || !strings.Contains(out, "cli/cmd/seshat/main.go") || !strings.Contains(out, "calls") {
		t.Fatalf("unexpected mermaid output:\n%s", out)
	}
}

func TestRenderDOT(t *testing.T) {
	out, err := Render(testGraph(), Options{Format: FormatDOT, Direction: DirectionDependsOn, MaxNodes: 10})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if !strings.Contains(out, "digraph seshat_dependencies") || strings.Contains(out, "main_test.go") {
		t.Fatalf("unexpected dot output:\n%s", out)
	}
}

func testGraph() model.FileDependencyGraph {
	return model.FileDependencyGraph{
		File: model.File{Path: "cli/cmd/seshat/main.go"},
		DependsOn: []model.FileDependency{{
			File:    model.File{Path: "cli/internal/localquery/query.go"},
			Reasons: []model.RelationType{model.RelationCalls},
		}},
		Dependents: []model.FileDependency{{
			File:    model.File{Path: "cli/cmd/seshat/main_test.go"},
			Reasons: []model.RelationType{model.RelationCalls},
		}},
	}
}
