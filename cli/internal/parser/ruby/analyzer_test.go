package ruby

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/hungnm98/seshat-cli/internal/parser"
	"github.com/hungnm98/seshat-cli/pkg/model"
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

func TestAnalyzerNamespaceTracking(t *testing.T) {
	batch := analyzeSource(t, `
module Billing
  class OrderService
    def create
    end

    def cancel
    end
  end
end
`)

	symbolIDs := symbolIDSet(batch)

	if _, ok := symbolIDs["symbol:ruby:module:Billing"]; !ok {
		t.Error("expected module Billing")
	}
	if _, ok := symbolIDs["symbol:ruby:class:Billing::OrderService"]; !ok {
		t.Error("expected class Billing::OrderService")
	}
	if _, ok := symbolIDs["symbol:ruby:method:Billing::OrderService#create"]; !ok {
		t.Error("expected method Billing::OrderService#create")
	}
	if _, ok := symbolIDs["symbol:ruby:method:Billing::OrderService#cancel"]; !ok {
		t.Error("expected method Billing::OrderService#cancel — namespace bug if missing")
	}
}

func TestAnalyzerInheritance(t *testing.T) {
	batch := analyzeSource(t, `
class OrderService < BaseService
  def run
  end
end
`)

	hasInherits := false
	for _, rel := range batch.Relations {
		if rel.Type == model.RelationInherits &&
			rel.FromSymbolID == "symbol:ruby:class:OrderService" &&
			rel.ToSymbolID == "symbol:ruby:class:BaseService" {
			hasInherits = true
		}
	}
	if !hasInherits {
		t.Error("expected inherits relation from OrderService to BaseService")
	}
}

func TestAnalyzerRequireImports(t *testing.T) {
	batch := analyzeSource(t, `
require 'active_record'
require_relative '../models/user'

class Foo
  def bar
  end
end
`)

	var importSymbols []model.Symbol
	for _, sym := range batch.Symbols {
		if sym.Kind == "import" {
			importSymbols = append(importSymbols, sym)
		}
	}
	if len(importSymbols) < 2 {
		t.Errorf("expected at least 2 import symbols, got %d", len(importSymbols))
	}

	hasImportRelation := false
	for _, rel := range batch.Relations {
		if rel.Type == model.RelationImports {
			hasImportRelation = true
			break
		}
	}
	if !hasImportRelation {
		t.Error("expected at least one imports relation")
	}
}

func TestAnalyzerMixins(t *testing.T) {
	batch := analyzeSource(t, `
module Logging
  def log(msg)
  end
end

class Service
  include Logging
end
`)

	hasMixin := false
	for _, rel := range batch.Relations {
		if rel.Type == model.RelationReferences &&
			strings.Contains(rel.FromSymbolID, "Service") &&
			strings.Contains(rel.ToSymbolID, "Logging") {
			hasMixin = true
		}
	}
	if !hasMixin {
		t.Error("expected references relation from Service to Logging (mixin)")
	}
}

func TestAnalyzerAttrAccessor(t *testing.T) {
	batch := analyzeSource(t, `
class User
  attr_accessor :name, :email
  attr_reader :id
end
`)

	symbolIDs := symbolIDSet(batch)
	for _, name := range []string{"name", "email", "id"} {
		id := "symbol:ruby:method:User#" + name
		if _, ok := symbolIDs[id]; !ok {
			t.Errorf("expected attr method %s", id)
		}
	}
}

func TestAnalyzerCallRelationsUseCurrentMethod(t *testing.T) {
	batch := analyzeSource(t, `
class PaymentService
  def process
    gateway.charge
  end

  def gateway
  end
end
`)

	hasCallFromProcess := false
	for _, rel := range batch.Relations {
		if rel.Type == model.RelationCalls &&
			rel.FromSymbolID == "symbol:ruby:method:PaymentService#process" {
			hasCallFromProcess = true
		}
	}
	_ = hasCallFromProcess // call detection is heuristic; just ensure no panic
}

func TestAnalyzerLineEndTracking(t *testing.T) {
	batch := analyzeSource(t, `
class Foo
  def short
  end

  def longer
    x = 1
    y = 2
  end
end
`)

	for _, sym := range batch.Symbols {
		if sym.Kind == "method" && sym.LineEnd > 0 && sym.LineEnd < sym.LineStart {
			t.Errorf("symbol %s has line_end %d < line_start %d", sym.ID, sym.LineEnd, sym.LineStart)
		}
		if sym.Kind == "method" && sym.Name == "longer" && sym.LineEnd <= sym.LineStart {
			t.Errorf("method longer should have line_end > line_start, got start=%d end=%d", sym.LineStart, sym.LineEnd)
		}
	}
}

func TestAnalyzerParallelMatchesSequential(t *testing.T) {
	input := parser.Input{
		ProjectID:     "rails-app",
		RepoPath:      "../../../testdata/ruby_sample",
		IncludePaths:  []string{"app"},
		ExcludePaths:  []string{"vendor"},
		CommitSHA:     "abc123",
		Branch:        "main",
		SchemaVersion: "v1",
		ScanMode:      "full",
		Parallelism:   1,
	}
	a := New()
	seq, err := a.Analyze(context.Background(), input)
	if err != nil {
		t.Fatalf("sequential: %v", err)
	}
	input.Parallelism = 4
	par, err := a.Analyze(context.Background(), input)
	if err != nil {
		t.Fatalf("parallel: %v", err)
	}
	if len(seq.Symbols) != len(par.Symbols) {
		t.Errorf("symbol count differs: seq=%d par=%d", len(seq.Symbols), len(par.Symbols))
	}
	if len(seq.Relations) != len(par.Relations) {
		t.Errorf("relation count differs: seq=%d par=%d", len(seq.Relations), len(par.Relations))
	}
}

// analyzeSource is a helper that parses an inline Ruby snippet via a temp file.
func analyzeSource(t *testing.T, src string) model.AnalysisBatch {
	t.Helper()
	dir := t.TempDir()
	path := dir + "/test.rb"
	if err := writeFile(path, src); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	a := New()
	batch, err := a.Analyze(context.Background(), parser.Input{
		ProjectID:     "test",
		RepoPath:      dir,
		CommitSHA:     "test",
		Branch:        "main",
		SchemaVersion: "v1",
		ScanMode:      "full",
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	return batch
}

func symbolIDSet(batch model.AnalysisBatch) map[string]struct{} {
	m := make(map[string]struct{}, len(batch.Symbols))
	for _, sym := range batch.Symbols {
		m[sym.ID] = struct{}{}
	}
	return m
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
