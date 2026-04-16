package localindex

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hungnm98/seshat-cli/pkg/model"
)

func TestWriteReadGraphAndStatus(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".seshat", "project.yaml")
	batch := model.AnalysisBatch{
		Metadata: model.GraphMetadata{
			ProjectID:     "proj",
			CommitSHA:     "abc",
			Branch:        "main",
			SchemaVersion: "v1",
			GeneratedAt:   time.Unix(10, 0).UTC(),
			ScanMode:      "full",
		},
		Files:     []model.File{{ID: "file:go:a.go", Path: "a.go", Language: "go"}},
		Symbols:   []model.Symbol{{ID: "symbol:go:main:func:Run", FileID: "file:go:a.go", Name: "Run"}},
		Relations: []model.Relation{{ID: "relation:1"}},
	}

	if err := WriteGraph(configPath, batch); err != nil {
		t.Fatalf("WriteGraph returned error: %v", err)
	}
	status := BuildStatus(configPath, dir, ConfigHash([]byte("config")), batch)
	if err := WriteStatus(configPath, status); err != nil {
		t.Fatalf("WriteStatus returned error: %v", err)
	}

	if _, err := os.Stat(GraphPath(configPath)); err != nil {
		t.Fatalf("graph file missing: %v", err)
	}
	gotBatch, err := ReadGraph(configPath)
	if err != nil {
		t.Fatalf("ReadGraph returned error: %v", err)
	}
	if gotBatch.Metadata.ProjectID != "proj" || len(gotBatch.Files) != 1 {
		t.Fatalf("unexpected graph: %#v", gotBatch)
	}
	gotStatus, err := ReadStatus(configPath)
	if err != nil {
		t.Fatalf("ReadStatus returned error: %v", err)
	}
	if gotStatus.FilesCount != 1 || gotStatus.LanguageCounts["go"] != 1 {
		t.Fatalf("unexpected status: %#v", gotStatus)
	}
}
