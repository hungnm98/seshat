package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hungnm98/seshat-cli/internal/config"
	"github.com/hungnm98/seshat-cli/internal/parser"
	goanalyzer "github.com/hungnm98/seshat-cli/internal/parser/golang"
	rubyanalyzer "github.com/hungnm98/seshat-cli/internal/parser/ruby"
	"github.com/hungnm98/seshat-cli/pkg/graphschema"
	"github.com/hungnm98/seshat-cli/pkg/model"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}
	switch os.Args[1] {
	case "scan":
		if err := runScan(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	case "auth":
		if err := runAuth(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	case "dependencies":
		if err := runDependencies(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	default:
		usage()
	}
}

func runScan(args []string) error {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	configPath := fs.String("config", ".seshat/project.yaml", "Path to project config")
	mode := fs.String("mode", "full", "Scan mode: full or incremental")
	if err := fs.Parse(args); err != nil {
		return err
	}
	projectCfg, err := config.LoadCLIProject(*configPath)
	if err != nil {
		return err
	}
	analyzers := map[string]parser.Analyzer{
		"go":   goanalyzer.New(),
		"ruby": rubyanalyzer.New(),
	}
	if err := parser.ValidateTargets(projectCfg.LanguageTargets, analyzers); err != nil {
		return err
	}

	commitSHA := gitValue(projectCfg.RepoPath, "rev-parse", "HEAD")
	branch := gitValue(projectCfg.RepoPath, "rev-parse", "--abbrev-ref", "HEAD")
	input := parser.Input{
		ProjectID:     projectCfg.ProjectID,
		RepoPath:      projectCfg.RepoPath,
		IncludePaths:  projectCfg.IncludePaths,
		ExcludePaths:  projectCfg.ExcludePaths,
		TargetFiles:   nil,
		CommitSHA:     commitSHA,
		Branch:        branch,
		SchemaVersion: graphschema.Version,
		ScanMode:      *mode,
	}
	if *mode == "incremental" {
		targetFiles, err := discoverChangedFiles(projectCfg.RepoPath)
		if err != nil {
			return err
		}
		if len(targetFiles) == 0 {
			fmt.Println("no changed files detected; skipping incremental upload")
			return nil
		}
		input.TargetFiles = targetFiles
	}
	var batches []model.AnalysisBatch
	for _, target := range projectCfg.LanguageTargets {
		batch, err := analyzers[target].Analyze(context.Background(), input)
		if err != nil {
			return err
		}
		batches = append(batches, batch)
	}
	merged := parser.MergeBatches(projectCfg.ProjectID, commitSHA, branch, graphschema.Version, *mode, batches...)
	payload, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return err
	}
	token := os.Getenv(projectCfg.TokenEnvVar)
	if token == "" {
		return fmt.Errorf("missing token in env var %s", projectCfg.TokenEnvVar)
	}
	endpoint := strings.TrimRight(projectCfg.ServerEndpoint, "/") + "/api/v1/projects/" + projectCfg.ProjectID + "/ingestions"
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("upload failed: %s", body)
	}
	fmt.Println(string(body))
	return nil
}

func runAuth(args []string) error {
	if len(args) == 0 || args[0] != "verify" {
		return errors.New("usage: seshat-cli auth verify --config .seshat/project.yaml")
	}
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	configPath := fs.String("config", ".seshat/project.yaml", "Path to project config")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	projectCfg, err := config.LoadCLIProject(*configPath)
	if err != nil {
		return err
	}
	token := os.Getenv(projectCfg.TokenEnvVar)
	if token == "" {
		return fmt.Errorf("missing token in env var %s", projectCfg.TokenEnvVar)
	}
	endpoint := fmt.Sprintf("%s/api/v1/auth/verify?project_id=%s", strings.TrimRight(projectCfg.ServerEndpoint, "/"), projectCfg.ProjectID)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("token verification failed: %s", body)
	}
	fmt.Println(string(body))
	return nil
}

func runDependencies(args []string) error {
	fs := flag.NewFlagSet("dependencies", flag.ExitOnError)
	configPath := fs.String("config", ".seshat/project.yaml", "Path to project config")
	filePath := fs.String("file", "", "Project-relative file path to inspect")
	depth := fs.Int("depth", 1, "Dependency traversal depth")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *filePath == "" {
		return errors.New("usage: seshat-cli dependencies --file internal/order/service.go --config .seshat/project.yaml")
	}
	projectCfg, err := config.LoadCLIProject(*configPath)
	if err != nil {
		return err
	}
	token := os.Getenv(projectCfg.TokenEnvVar)
	if token == "" {
		return fmt.Errorf("missing token in env var %s", projectCfg.TokenEnvVar)
	}
	endpoint := fmt.Sprintf(
		"%s/api/v1/projects/%s/graph/dependencies?file=%s&depth=%d",
		strings.TrimRight(projectCfg.ServerEndpoint, "/"),
		projectCfg.ProjectID,
		url.QueryEscape(*filePath),
		*depth,
	)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("dependency graph query failed: %s", body)
	}
	fmt.Println(string(body))
	return nil
}

func gitValue(repo string, args ...string) string {
	cmd := exec.Command("git", append([]string{"-C", filepath.Clean(repo)}, args...)...)
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

func usage() {
	fmt.Println("Usage:")
	fmt.Println("  seshat-cli scan --config .seshat/project.yaml --mode full")
	fmt.Println("  seshat-cli auth verify --config .seshat/project.yaml")
	fmt.Println("  seshat-cli dependencies --config .seshat/project.yaml --file internal/order/service.go --depth 1")
}

func discoverChangedFiles(repoPath string) ([]string, error) {
	if _, err := exec.Command("git", "-C", filepath.Clean(repoPath), "rev-parse", "--is-inside-work-tree").Output(); err != nil {
		return nil, fmt.Errorf("discover changed files: %w", err)
	}

	candidates := make(map[string]struct{})
	diffCmd := exec.Command("git", "-C", filepath.Clean(repoPath), "diff", "--name-only", "--diff-filter=ACMRTUXB", "HEAD")
	diffOutput, err := diffCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("discover changed files diff: %w", err)
	}
	for _, line := range strings.Split(string(diffOutput), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		candidates[line] = struct{}{}
	}

	untrackedCmd := exec.Command("git", "-C", filepath.Clean(repoPath), "ls-files", "--others", "--exclude-standard")
	untrackedOutput, err := untrackedCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("discover changed files untracked: %w", err)
	}
	for _, line := range strings.Split(string(untrackedOutput), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		candidates[line] = struct{}{}
	}

	files := make([]string, 0, len(candidates))
	for file := range candidates {
		files = append(files, filepath.Clean(file))
	}
	sort.Strings(files)
	return files, nil
}
