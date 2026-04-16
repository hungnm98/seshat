package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

func TestLocalCoreCLISmoke(t *testing.T) {
	repo := t.TempDir()
	mustRun(t, repo, "git", "init")
	mustRun(t, repo, "git", "config", "user.email", "test@example.com")
	mustRun(t, repo, "git", "config", "user.name", "Test User")
	writeFile(t, repo, "internal/order/service.go", `package order

func CreateOrder() {
	Validate()
}

func Validate() {}
`)
	mustRun(t, repo, "git", "add", ".")
	mustRun(t, repo, "git", "-c", "commit.gpgsign=false", "commit", "-m", "initial")

	configPath := filepath.Join(repo, ".seshat", "project.yaml")
	var stdout, stderr bytes.Buffer
	if err := run([]string{"init", "--repo", repo, "--config", configPath, "--project-id", "proj"}, &stdout, &stderr); err != nil {
		t.Fatalf("init failed: %v\nstderr=%s", err, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if err := run([]string{"ingest", "--config", configPath}, &stdout, &stderr); err != nil {
		t.Fatalf("ingest failed: %v\nstderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "symbols") {
		t.Fatalf("expected ingest summary, got %s", stdout.String())
	}
	stdout.Reset()
	if err := run([]string{"status", "--config", configPath, "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if !strings.Contains(stdout.String(), `"changed_files_count"`) {
		t.Fatalf("expected status json, got %s", stdout.String())
	}
	stdout.Reset()
	if err := run([]string{"inspect", "--config", configPath, "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("inspect failed: %v", err)
	}
	if !strings.Contains(stdout.String(), `"symbols_count"`) {
		t.Fatalf("expected inspect json, got %s", stdout.String())
	}
	stdout.Reset()
	if err := run([]string{"graph", "--config", configPath, "--file", "internal/order/service.go", "--format", "mermaid"}, &stdout, &stderr); err != nil {
		t.Fatalf("graph failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "flowchart LR") || !strings.Contains(stdout.String(), "internal/order/service.go") {
		t.Fatalf("expected mermaid graph, got %s", stdout.String())
	}
}

func TestLocalMCPCommandSmoke(t *testing.T) {
	repo := t.TempDir()
	mustRun(t, repo, "git", "init")
	mustRun(t, repo, "git", "config", "user.email", "test@example.com")
	mustRun(t, repo, "git", "config", "user.name", "Test User")
	writeFile(t, repo, "main.go", `package main

func Run() {}
`)
	mustRun(t, repo, "git", "add", ".")
	mustRun(t, repo, "git", "-c", "commit.gpgsign=false", "commit", "-m", "initial")
	configPath := filepath.Join(repo, ".seshat", "project.yaml")
	var stdout, stderr bytes.Buffer
	if err := run([]string{"init", "--repo", repo, "--config", configPath, "--project-id", "proj"}, &stdout, &stderr); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	stdout.Reset()
	if err := run([]string{"ingest", "--config", configPath}, &stdout, &stderr); err != nil {
		t.Fatalf("ingest failed: %v", err)
	}
	stdout.Reset()
	input := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"find_symbol","arguments":{"project_id":"proj","query":"Run"}}}`,
	}, "\n") + "\n")
	if err := runMCP([]string{"--config", configPath}, input, &stdout); err != nil {
		t.Fatalf("mcp failed: %v", err)
	}
	if !strings.Contains(stdout.String(), `"find_symbol"`) || !strings.Contains(stdout.String(), `"Run"`) {
		t.Fatalf("unexpected mcp output: %s", stdout.String())
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeFile(%s): %v", rel, err)
	}
}
