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
	"time"

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
	registryPath := filepath.Join(repo, ".seshat", "config.yml")
	if err := run([]string{"mcp", "add", repo, "--registry", registryPath}, &stdout, &stderr); err != nil {
		t.Fatalf("mcp add failed: %v", err)
	}
	stdout.Reset()
	input := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"find_symbol","arguments":{"project_id":"proj","query":"Run"}}}`,
	}, "\n") + "\n")
	if err := runMCP([]string{"--registry", registryPath}, input, &stdout); err != nil {
		t.Fatalf("mcp failed: %v", err)
	}
	if !strings.Contains(stdout.String(), `"find_symbol"`) || !strings.Contains(stdout.String(), `"Run"`) {
		t.Fatalf("unexpected mcp output: %s", stdout.String())
	}
}

func TestMCPHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"mcp", "-h"}, &stdout, &stderr); err != nil {
		t.Fatalf("mcp -h failed: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		"seshat mcp [--registry",
		"seshat mcp add {folder_project}",
		"seshat mcp reload",
		"seshat mcp project ls",
		"seshat mcp config path|show|edit",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected help to contain %q, got %s", want, out)
		}
	}
}

func TestVersionCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"version"}, &stdout, &stderr); err != nil {
		t.Fatalf("version failed: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "seshat 0.2.0" {
		t.Fatalf("unexpected version output: %q", got)
	}
}

func TestMCPAddProjectListAndConfigCommands(t *testing.T) {
	root := t.TempDir()
	registryPath := filepath.Join(root, "config.yml")
	alpha := filepath.Join(root, "alpha")
	beta := filepath.Join(root, "beta")
	writeProjectConfig(t, alpha, "alpha")
	writeProjectConfig(t, beta, "beta")

	var stdout, stderr bytes.Buffer
	if err := run([]string{"mcp", "add", beta, "--registry", registryPath}, &stdout, &stderr); err != nil {
		t.Fatalf("mcp add beta failed: %v\nstderr=%s", err, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if err := run([]string{"mcp", "add", "--registry=" + registryPath, alpha, "--reload"}, &stdout, &stderr); err != nil {
		t.Fatalf("mcp add alpha failed: %v\nstderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "lazily reloads registry and graph files on its next tool call") {
		t.Fatalf("expected lazy reload message, got %s", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if err := run([]string{"mcp", "reload", "--registry", registryPath}, &stdout, &stderr); err != nil {
		t.Fatalf("mcp reload failed: %v\nstderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "lazily reloads registry and graph files on its next tool call") {
		t.Fatalf("expected lazy reload message, got %s", stdout.String())
	}
	stdout.Reset()
	if err := run([]string{"mcp", "project", "ls", "--registry", registryPath, "--sort", "id"}, &stdout, &stderr); err != nil {
		t.Fatalf("mcp project ls failed: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two projects, got %q", stdout.String())
	}
	if !strings.HasPrefix(lines[0], "alpha\t") || !strings.HasPrefix(lines[1], "beta\t") {
		t.Fatalf("expected sorted projects, got %q", stdout.String())
	}
	stdout.Reset()
	if err := run([]string{"mcp", "config", "path", "--registry", registryPath}, &stdout, &stderr); err != nil {
		t.Fatalf("mcp config path failed: %v", err)
	}
	if strings.TrimSpace(stdout.String()) != registryPath {
		t.Fatalf("unexpected config path output: %q", stdout.String())
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
	registryPath := filepath.Join(repo, ".seshat", "config.yml")
	if err := writeMCPRegistry(registryPath, mcpRegistryConfig{Projects: []mcpProject{
		{ID: "proj", Path: repo, Config: configPath},
	}}); err != nil {
		t.Fatalf("writeMCPRegistry returned error: %v", err)
	}

	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()
	done := make(chan error, 1)
	go func() {
		err := runMCP([]string{"--registry", registryPath}, stdinReader, stdoutWriter)
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

func TestRegistryMCPRoutesProjectsAndListsStatus(t *testing.T) {
	root := t.TempDir()
	registryPath := filepath.Join(root, "config.yml")
	alpha := filepath.Join(root, "alpha")
	beta := filepath.Join(root, "beta")
	writeProjectConfig(t, alpha, "alpha")
	writeProjectConfig(t, beta, "beta")
	alphaConfig := filepath.Join(alpha, ".seshat", "project.yaml")
	betaConfig := filepath.Join(beta, ".seshat", "project.yaml")
	writeNamedGraph(t, alphaConfig, "alpha", "AlphaRun")
	writeNamedGraph(t, betaConfig, "beta", "BetaRun")
	if err := localindex.WriteStatus(alphaConfig, localindex.Status{
		ProjectID:      "alpha",
		ConfigPath:     alphaConfig,
		RepoPath:       alpha,
		GeneratedAt:    modelTime(),
		FilesCount:     2,
		SymbolsCount:   3,
		RelationsCount: 4,
	}); err != nil {
		t.Fatalf("WriteStatus returned error: %v", err)
	}
	registry := mcpRegistryConfig{Projects: []mcpProject{
		{ID: "beta", Path: beta, Config: betaConfig},
		{ID: "alpha", Path: alpha, Config: alphaConfig},
	}}
	if err := writeMCPRegistry(registryPath, registry); err != nil {
		t.Fatalf("writeMCPRegistry returned error: %v", err)
	}

	var stdout bytes.Buffer
	input := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_projects","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"find_symbol","arguments":{"project_id":"alpha","query":"AlphaRun"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"find_symbol","arguments":{"project_id":"beta","query":"BetaRun"}}}`,
	}, "\n") + "\n")
	if err := runMCP([]string{"--registry", registryPath}, input, &stdout); err != nil {
		t.Fatalf("mcp registry failed: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, `"project_id":"alpha"`) || !strings.Contains(out, `"status":"indexed"`) || !strings.Contains(out, `"files_count":2`) {
		t.Fatalf("expected indexed alpha in list_projects, got %s", out)
	}
	if !strings.Contains(out, `"project_id":"beta"`) || !strings.Contains(out, `"status":"not_indexed"`) || !strings.Contains(out, "run seshat scan") {
		t.Fatalf("expected not_indexed beta in list_projects, got %s", out)
	}
	if !strings.Contains(out, "AlphaRun") || !strings.Contains(out, "BetaRun") {
		t.Fatalf("expected project-routed symbols, got %s", out)
	}
}

func TestMCPRegistryRejectsDuplicateProjectID(t *testing.T) {
	root := t.TempDir()
	registryPath := filepath.Join(root, "config.yml")
	writeFile(t, root, "config.yml", `projects:
  - id: dup
    path: /repo/a
    config: /repo/a/.seshat/project.yaml
  - id: dup
    path: /repo/b
    config: /repo/b/.seshat/project.yaml
`)
	if _, err := loadMCPRegistry(registryPath); err == nil || !strings.Contains(err.Error(), "duplicate project id") {
		t.Fatalf("expected duplicate project id error, got %v", err)
	}
}

func TestRegistryMCPDoesNotRouteMismatchedGraphProjectID(t *testing.T) {
	root := t.TempDir()
	registryPath := filepath.Join(root, "config.yml")
	repo := filepath.Join(root, "repo")
	writeProjectConfig(t, repo, "declared")
	configPath := filepath.Join(repo, ".seshat", "project.yaml")
	writeNamedGraph(t, configPath, "other", "Run")
	if err := writeMCPRegistry(registryPath, mcpRegistryConfig{Projects: []mcpProject{
		{ID: "declared", Path: repo, Config: configPath},
	}}); err != nil {
		t.Fatalf("writeMCPRegistry returned error: %v", err)
	}
	var stdout bytes.Buffer
	input := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"find_symbol","arguments":{"project_id":"declared","query":"Run"}}}` + "\n")
	if err := runMCP([]string{"--registry", registryPath}, input, &stdout); err != nil {
		t.Fatalf("mcp registry failed: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "invalid project_id") || strings.Contains(out, `"Run"`) {
		t.Fatalf("expected graph project mismatch error without routed results, got %s", out)
	}
}

func TestRegistryProviderEvictsIdleQueries(t *testing.T) {
	repo := t.TempDir()
	configPath := filepath.Join(repo, ".seshat", "project.yaml")
	writeProjectConfig(t, repo, "proj")
	writeNamedGraph(t, configPath, "proj", "Run")
	provider := &reloadingQueryProvider{configPath: configPath, projectID: "proj"}
	if _, err := provider.Query(); err != nil {
		t.Fatalf("Query returned error: %v", err)
	}
	if provider.query == nil {
		t.Fatal("expected query to be cached")
	}
	provider.EvictIdle(provider.lastUsed.Add(time.Minute), 30*time.Second)
	if provider.query != nil || provider.size != 0 || !provider.modTime.IsZero() {
		t.Fatalf("expected idle query to be evicted: %#v", provider)
	}
}

func writeProjectConfig(t *testing.T, repo, projectID string) {
	t.Helper()
	writeFile(t, repo, ".seshat/project.yaml", fmt.Sprintf(`project_id: %s
repo_path: %s
language_targets:
  - go
include_paths: []
exclude_paths: []
`, projectID, repo))
}

func writeNamedGraph(t *testing.T, configPath, projectID, symbolName string) {
	t.Helper()
	err := localindex.WriteGraph(configPath, model.AnalysisBatch{
		Metadata: model.GraphMetadata{ProjectID: projectID},
		Symbols: []model.Symbol{
			{ID: "symbol:go:main:func:" + symbolName, Kind: "function", Name: symbolName, Path: "main.go"},
		},
	})
	if err != nil {
		t.Fatalf("WriteGraph returned error: %v", err)
	}
}

func modelTime() time.Time {
	return time.Unix(100, 0).UTC()
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
