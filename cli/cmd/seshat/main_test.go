package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hungnm98/seshat-cli/internal/localindex"
	"github.com/hungnm98/seshat-cli/pkg/model"
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
	if err := run([]string{"ingest", "--config", configPath, "--parallel", "2", "-v"}, &stdout, &stderr); err != nil {
		t.Fatalf("ingest failed: %v\nstderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "symbols") {
		t.Fatalf("expected ingest summary, got %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "parallel=2") || !strings.Contains(stderr.String(), "target=go done") {
		t.Fatalf("expected verbose ingest logs, got %s", stderr.String())
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

func TestMCPCommandReloadsUpdatedLocalIndex(t *testing.T) {
	repo := t.TempDir()
	configPath := filepath.Join(repo, ".seshat", "project.yaml")
	writeFile(t, repo, ".seshat/project.yaml", fmt.Sprintf(`project_id: proj
repo_path: %s
language_targets:
  - go
include_paths: []
exclude_paths: []
`, repo))
	writeMCPReloadGraph(t, configPath, "RunBefore")

	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()
	done := make(chan error, 1)
	go func() {
		err := runMCP([]string{"--config", configPath}, stdinReader, stdoutWriter)
		_ = stdoutWriter.Close()
		done <- err
	}()
	scanner := bufio.NewScanner(stdoutReader)
	call := func(id int, query string) string {
		t.Helper()
		_, err := fmt.Fprintf(stdinWriter, `{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"find_symbol","arguments":{"project_id":"proj","query":%q}}}`+"\n", id, query)
		if err != nil {
			t.Fatalf("write MCP request: %v", err)
		}
		if !scanner.Scan() {
			t.Fatalf("read MCP response failed: %v", scanner.Err())
		}
		return scanner.Text()
	}

	first := call(1, "RunBefore")
	if !strings.Contains(first, "RunBefore") {
		t.Fatalf("expected first response to use initial index, got %s", first)
	}
	writeMCPReloadGraph(t, configPath, "RunAfterReload")
	second := call(2, "RunAfterReload")
	if !strings.Contains(second, "RunAfterReload") {
		t.Fatalf("expected second response to use updated index, got %s", second)
	}

	_ = stdinWriter.Close()
	if err := <-done; err != nil {
		t.Fatalf("mcp failed: %v", err)
	}
}

func writeMCPReloadGraph(t *testing.T, configPath, symbolName string) {
	t.Helper()
	err := localindex.WriteGraph(configPath, model.AnalysisBatch{
		Metadata: model.GraphMetadata{ProjectID: "proj"},
		Symbols: []model.Symbol{
			{ID: "symbol:go:main:func:" + symbolName, Kind: "function", Name: symbolName, Path: "main.go"},
		},
	})
	if err != nil {
		t.Fatalf("WriteGraph returned error: %v", err)
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
