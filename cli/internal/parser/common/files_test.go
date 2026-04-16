package common

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCollectFilesFromCandidates(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "app/main.go", "package main\n")
	mustWrite(t, root, "app/service.go", "package app\n")
	mustWrite(t, root, "app/skip.rb", "module Skip\nend\n")
	mustWrite(t, root, "vendor/ignored.go", "package ignored\n")

	files, err := CollectFilesFromCandidates(root, []string{"app/main.go", "app/service.go", "app/skip.rb", "missing.go", "vendor/ignored.go"}, []string{"app"}, []string{"vendor"}, map[string]struct{}{".go": {}})
	if err != nil {
		t.Fatalf("CollectFilesFromCandidates returned error: %v", err)
	}
	want := []string{"app/main.go", "app/service.go"}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("unexpected files: got %v want %v", files, want)
	}
}

func mustWrite(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", rel, err)
	}
}
