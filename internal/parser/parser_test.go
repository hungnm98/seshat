package parser

import (
	"testing"

	"github.com/hungnm98/seshat/pkg/model"
)

func TestMergeBatchesDedupesByID(t *testing.T) {
	batchA := model.AnalysisBatch{
		Files:     []model.File{{ID: "file:a", Path: "a.go", Language: "go", Checksum: "sum-a"}},
		Symbols:   []model.Symbol{{ID: "symbol:a", FileID: "file:a", Kind: "function", Name: "A", Language: "go", Path: "a.go"}},
		Relations: []model.Relation{{ID: "relation:a", ProjectID: "proj", FromSymbolID: "symbol:a", ToSymbolID: "symbol:a", Type: model.RelationContains}},
	}
	batchB := model.AnalysisBatch{
		Files:     []model.File{{ID: "file:a", Path: "a.go", Language: "go", Checksum: "sum-a"}},
		Symbols:   []model.Symbol{{ID: "symbol:a", FileID: "file:a", Kind: "function", Name: "A", Language: "go", Path: "a.go"}},
		Relations: []model.Relation{{ID: "relation:a", ProjectID: "proj", FromSymbolID: "symbol:a", ToSymbolID: "symbol:a", Type: model.RelationContains}},
	}

	merged := MergeBatches("proj", "commit", "main", "v1", "incremental", batchA, batchB)

	if got := len(merged.Files); got != 1 {
		t.Fatalf("expected 1 file, got %d", got)
	}
	if got := len(merged.Symbols); got != 1 {
		t.Fatalf("expected 1 symbol, got %d", got)
	}
	if got := len(merged.Relations); got != 1 {
		t.Fatalf("expected 1 relation, got %d", got)
	}
	if merged.Metadata.ScanMode != "incremental" {
		t.Fatalf("expected scan mode incremental, got %s", merged.Metadata.ScanMode)
	}
}
