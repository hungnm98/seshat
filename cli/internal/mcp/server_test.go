package mcp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hungnm98/seshat-cli/internal/localquery"
	"github.com/hungnm98/seshat-cli/pkg/model"
)

func TestServerListsAndCallsTools(t *testing.T) {
	query, err := localquery.New("proj", model.AnalysisBatch{
		Metadata: model.GraphMetadata{ProjectID: "proj", CommitSHA: "abc", Branch: "main", SchemaVersion: "v1", GeneratedAt: time.Unix(10, 0).UTC()},
		Symbols:  []model.Symbol{{ID: "symbol:go:main:func:Run", Kind: "function", Name: "Run", Path: "main.go"}},
	})
	if err != nil {
		t.Fatalf("localquery.New returned error: %v", err)
	}
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"find_symbol","arguments":{"project_id":"proj","query":"Run"}}}`,
	}, "\n") + "\n"
	var out bytes.Buffer

	if err := NewServer(query).Serve(strings.NewReader(input), &out); err != nil {
		t.Fatalf("Serve returned error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 responses, got %d: %s", len(lines), out.String())
	}
	var callResp struct {
		Result struct {
			StructuredContent struct {
				Results []model.Symbol `json:"results"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[2]), &callResp); err != nil {
		t.Fatalf("parse call response: %v", err)
	}
	if len(callResp.Result.StructuredContent.Results) != 1 || callResp.Result.StructuredContent.Results[0].Name != "Run" {
		t.Fatalf("unexpected tool call response: %s", lines[2])
	}
}

func TestServerRejectsUnknownMethod(t *testing.T) {
	query, err := localquery.New("proj", model.AnalysisBatch{Metadata: model.GraphMetadata{ProjectID: "proj"}})
	if err != nil {
		t.Fatalf("localquery.New returned error: %v", err)
	}
	var out bytes.Buffer
	if err := NewServer(query).Serve(strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"missing"}`+"\n"), &out); err != nil {
		t.Fatalf("Serve returned error: %v", err)
	}
	if !strings.Contains(out.String(), `"code":-32601`) {
		t.Fatalf("expected method not found error, got %s", out.String())
	}
}

func TestTrimGraphLimitsDirectionAndCompactsByDefault(t *testing.T) {
	graph := model.FileDependencyGraph{
		File:    model.File{Path: "controllers/order/place_order.go"},
		Symbols: []model.Symbol{{ID: "symbol:controller"}},
		DependsOn: []model.FileDependency{
			{
				File:      model.File{Path: "services/order/create.go"},
				Symbols:   []model.Symbol{{ID: "symbol:service"}},
				Relations: []model.Relation{{ID: "relation:imports"}},
				Depth:     1,
				Reasons:   []model.RelationType{model.RelationImports},
			},
			{File: model.File{Path: "services/order/validate.go"}, Depth: 1},
		},
		Dependents: []model.FileDependency{
			{File: model.File{Path: "controllers/order/preview_order.go"}, Depth: 1},
		},
		Relations: []model.Relation{{ID: "relation:top"}},
	}

	trimmed := trimGraph(graph, "depends-on", 1, true)
	if len(trimmed.DependsOn) != 1 || trimmed.DependsOn[0].File.Path != "services/order/create.go" {
		t.Fatalf("unexpected depends_on trim: %#v", trimmed.DependsOn)
	}
	if trimmed.Dependents != nil {
		t.Fatalf("expected dependents to be omitted, got %#v", trimmed.Dependents)
	}
	if trimmed.Symbols != nil || trimmed.Relations != nil {
		t.Fatalf("expected compact top-level graph, got symbols=%#v relations=%#v", trimmed.Symbols, trimmed.Relations)
	}
	if trimmed.DependsOn[0].Symbols != nil || trimmed.DependsOn[0].Relations != nil {
		t.Fatalf("expected compact dependency, got %#v", trimmed.DependsOn[0])
	}
}
