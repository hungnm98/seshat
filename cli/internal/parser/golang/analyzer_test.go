package golang

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hungnm98/seshat-cli/internal/parser"
	"github.com/hungnm98/seshat-cli/pkg/model"
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

func TestAnalyzerResolvesNestedSelectorMethodHeuristically(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, repo, "services/order/create.go", `package order

type orderService struct{}

func (s *orderService) CreateOrder() {}
`)
	writeFile(t, repo, "controllers/order/place_order.go", `package order

type Services struct {
	OrderService any
}

type OrderController struct {
	services Services
}

func (c *OrderController) PlaceOrder() {
	c.services.OrderService.CreateOrder()
}
`)

	analyzer := New()
	batch, err := analyzer.Analyze(context.Background(), parser.Input{
		ProjectID:     "dax-be",
		RepoPath:      repo,
		IncludePaths:  []string{"services", "controllers"},
		SchemaVersion: "v1",
		ScanMode:      "full",
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	from := "symbol:go:order@controllers/order:method:OrderController.PlaceOrder"
	to := "symbol:go:order@services/order:method:orderService.CreateOrder"
	for _, relation := range batch.Relations {
		if relation.Type != model.RelationCalls || relation.FromSymbolID != from || relation.ToSymbolID != to {
			continue
		}
		if relation.Metadata["resolution"] != "heuristic_selector_method" {
			t.Fatalf("expected heuristic metadata, got %#v", relation.Metadata)
		}
		if relation.Metadata["selector"] != "c.services.OrderService.CreateOrder" {
			t.Fatalf("expected selector metadata, got %#v", relation.Metadata)
		}
		return
	}
	t.Fatalf("expected call relation %s -> %s, got %#v", from, to, batch.Relations)
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
}
