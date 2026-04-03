package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDiscoverChangedFiles(t *testing.T) {
	repo := t.TempDir()

	mustRun(t, repo, "git", "init")
	mustRun(t, repo, "git", "config", "user.email", "test@example.com")
	mustRun(t, repo, "git", "config", "user.name", "Test User")

	writeFile(t, repo, "tracked.go", "package main\n")
	writeFile(t, repo, "ignored.txt", "ignore me\n")
	mustRun(t, repo, "git", "add", "tracked.go")
	mustRun(t, repo, "git", "-c", "commit.gpgsign=false", "commit", "-m", "initial")

	writeFile(t, repo, "tracked.go", "package main\n\nfunc main() {}\n")
	writeFile(t, repo, "new.go", "package main\n")

	files, err := discoverChangedFiles(repo)
	if err != nil {
		t.Fatalf("discoverChangedFiles returned error: %v", err)
	}

	want := []string{"ignored.txt", "new.go", "tracked.go"}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("unexpected changed files: got %v want %v", files, want)
	}
}

func mustRun(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := execCommand(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, string(out))
	}
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeFile(%s): %v", rel, err)
	}
}
