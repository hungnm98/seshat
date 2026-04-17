package main

import (
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
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hungnm98/seshat-cli/internal/config"
	"github.com/hungnm98/seshat-cli/internal/graphrender"
	"github.com/hungnm98/seshat-cli/internal/localindex"
	"github.com/hungnm98/seshat-cli/internal/localquery"
	mcpserver "github.com/hungnm98/seshat-cli/internal/mcp"
	"github.com/hungnm98/seshat-cli/internal/parser"
	goanalyzer "github.com/hungnm98/seshat-cli/internal/parser/golang"
	rubyanalyzer "github.com/hungnm98/seshat-cli/internal/parser/ruby"
	"github.com/hungnm98/seshat-cli/internal/setup"
	"github.com/hungnm98/seshat-cli/internal/watch"
	"github.com/hungnm98/seshat-cli/pkg/graphschema"
	"github.com/hungnm98/seshat-cli/pkg/model"
	"gopkg.in/yaml.v3"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		log.Fatal(err)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) < 1 {
		usage(stdout)
		return nil
	}
	switch args[0] {
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "ingest":
		return runIngest(args[1:], stdout, stderr)
	case "scan":
		fmt.Fprintln(stderr, "`scan` is deprecated in CLI-first mode; use `ingest` instead.")
		return runIngest(args[1:], stdout, stderr)
	case "push":
		return runPush(args[1:], stdout, stderr)
	case "watch":
		return runWatch(args[1:], stdout, stderr)
	case "inspect":
		return runInspect(args[1:], stdout)
	case "status":
		return runStatus(args[1:], stdout)
	case "mcp":
		return runMCP(args[1:], os.Stdin, stdout)
	case "graph":
		return runGraph(args[1:], stdout, stderr)
	case "setup":
		return runSetup(args[1:], stdout)
	case "auth":
		return runAuth(args[1:], stdout)
	case "dependencies":
		return runDependencies(args[1:], stdout)
	default:
		usage(stdout)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runInit(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", ".seshat/project.yaml", "Path to project config")
	projectID := fs.String("project-id", "", "Project id to write into config")
	repoPath := fs.String("repo", ".", "Repository path to index")
	force := fs.Bool("force", false, "Overwrite existing config")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if _, err := os.Stat(*configPath); err == nil && !*force {
		return fmt.Errorf("%s already exists; pass --force to overwrite", *configPath)
	}
	repoAbs, err := filepath.Abs(*repoPath)
	if err != nil {
		return err
	}
	detected, includePaths, err := detectProject(repoAbs)
	if err != nil {
		return err
	}
	id := strings.TrimSpace(*projectID)
	if id == "" {
		id = projectIDFromPath(repoAbs)
	}
	cfg := config.CLIProjectConfig{
		ProjectID:       id,
		RepoPath:        repoAbs,
		LanguageTargets: detected,
		IncludePaths:    includePaths,
		ExcludePaths:    []string{"vendor", "node_modules", "dist", "build", "tmp", "coverage", ".seshat/index"},
		ServerEndpoint:  "http://localhost:8080",
		TokenEnvVar:     "SESHAT_PROJECT_TOKEN",
	}
	if len(cfg.LanguageTargets) == 0 {
		cfg.LanguageTargets = []string{"go", "ruby"}
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Clean(*configPath)), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(*configPath, data, 0o644); err != nil {
		return fmt.Errorf("write project config: %w", err)
	}
	if err := localindex.Ensure(*configPath); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "initialized Seshat project %q at %s\n", cfg.ProjectID, *configPath)
	return nil
}

func runIngest(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("ingest", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", ".seshat/project.yaml", "Path to project config")
	all := fs.Bool("all", false, "Enable all local analysis features currently supported")
	dryRun := fs.Bool("dry-run", false, "Parse and print output without writing local index")
	jsonOut := fs.Bool("json", false, "Print JSON output")
	parallelism := fs.Int("parallel", 1, "Number of files to parse concurrently")
	threads := fs.Int("threads", 0, "Alias for --parallel")
	verbose := false
	fs.BoolVar(&verbose, "v", false, "Print verbose ingest logs")
	fs.BoolVar(&verbose, "verbose", false, "Print verbose ingest logs")
	if err := fs.Parse(args); err != nil {
		return err
	}
	_ = all
	if *threads > 0 {
		*parallelism = *threads
	}
	if *parallelism < 1 {
		return fmt.Errorf("parallel must be >= 1")
	}
	fmt.Fprintf(stderr, "ingest: scanning repository from %s with parallel=%d\n", *configPath, *parallelism)
	batch, cfg, configHash, err := buildBatchWithOptions(*configPath, "full", nil, buildOptions{
		Parallelism: *parallelism,
		Verbose:     verbose,
		Log:         stderr,
	})
	if err != nil {
		return err
	}
	summary := localindex.Summarize(batch)
	fmt.Fprintf(stderr, "ingest: analyzed %d files, %d symbols, %d relations\n", summary.FilesCount, summary.SymbolsCount, summary.RelationsCount)
	if !*dryRun {
		fmt.Fprintf(stderr, "ingest: writing graph index\n")
		if err := localindex.WriteGraph(*configPath, batch); err != nil {
			return err
		}
		fmt.Fprintf(stderr, "ingest: writing status index\n")
		if err := localindex.WriteStatus(*configPath, localindex.BuildStatus(*configPath, cfg.RepoPath, configHash, batch)); err != nil {
			return err
		}
		fmt.Fprintf(stderr, "ingest: index updated\n")
	}
	if *jsonOut {
		return printJSON(stdout, batch)
	}
	printSummary(stdout, "ingest", summary, *dryRun)
	return nil
}

func runPush(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("push", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", ".seshat/project.yaml", "Path to project config")
	force := fs.Bool("force", false, "Re-index the full repository")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *force {
		batch, cfg, configHash, err := buildBatch(*configPath, "full", nil)
		if err != nil {
			return err
		}
		if err := localindex.WriteGraph(*configPath, batch); err != nil {
			return err
		}
		if err := localindex.WriteStatus(*configPath, localindex.BuildStatus(*configPath, cfg.RepoPath, configHash, batch)); err != nil {
			return err
		}
		printSummary(stdout, "push --force", localindex.Summarize(batch), false)
		return nil
	}
	cfg, configHash, err := loadConfigWithHash(*configPath)
	if err != nil {
		return err
	}
	changed, err := discoverChangedFiles(cfg.RepoPath)
	if err != nil {
		return err
	}
	changed = indexableChangedFiles(changed, cfg)
	if len(changed) == 0 {
		fmt.Fprintln(stdout, "no changed source files detected; local index unchanged")
		return nil
	}
	current, err := localindex.ReadGraph(*configPath)
	if err != nil {
		return err
	}
	delta, _, _, err := buildBatch(*configPath, "incremental", changed)
	if err != nil {
		return err
	}
	merged := mergeIncremental(current, delta, changed)
	if err := localindex.WriteGraph(*configPath, merged); err != nil {
		return err
	}
	if err := localindex.WriteStatus(*configPath, localindex.BuildStatus(*configPath, cfg.RepoPath, configHash, merged)); err != nil {
		return err
	}
	printSummary(stdout, "push", localindex.Summarize(merged), false)
	return nil
}

func runWatch(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", ".seshat/project.yaml", "Path to project config")
	debounceMS := fs.Int("debounce", 2000, "Polling/debounce interval in milliseconds")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, _, err := loadConfigWithHash(*configPath)
	if err != nil {
		return err
	}
	interval := time.Duration(*debounceMS) * time.Millisecond
	if interval <= 0 {
		interval = 2 * time.Second
	}
	previous, err := watch.SnapshotFiles(cfg.RepoPath, cfg.IncludePaths, cfg.ExcludePaths)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "watching %s every %s; press Ctrl+C to stop\n", cfg.RepoPath, interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		next, err := watch.SnapshotFiles(cfg.RepoPath, cfg.IncludePaths, cfg.ExcludePaths)
		if err != nil {
			fmt.Fprintf(stderr, "watch snapshot failed: %v\n", err)
			continue
		}
		changed := watch.Changed(previous, next)
		if len(changed) == 0 {
			continue
		}
		previous = next
		if err := runPush([]string{"--config", *configPath}, stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "push failed: %v\n", err)
		}
	}
	return nil
}

func runInspect(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	configPath := fs.String("config", ".seshat/project.yaml", "Path to project config")
	jsonOut := fs.Bool("json", false, "Print JSON output")
	reparse := fs.Bool("reparse", false, "Inspect freshly parsed metadata instead of saved index")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var batch model.AnalysisBatch
	var err error
	if *reparse {
		batch, _, _, err = buildBatch(*configPath, "inspect", nil)
	} else {
		batch, err = localindex.ReadGraph(*configPath)
	}
	if err != nil {
		return err
	}
	summary := localindex.Summarize(batch)
	if *jsonOut {
		return printJSON(stdout, map[string]any{"summary": summary, "metadata": batch.Metadata})
	}
	printSummary(stdout, "inspect", summary, false)
	return nil
}

func runStatus(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	configPath := fs.String("config", ".seshat/project.yaml", "Path to project config")
	jsonOut := fs.Bool("json", false, "Print JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, _, err := loadConfigWithHash(*configPath)
	if err != nil {
		return err
	}
	status, err := localindex.ReadStatus(*configPath)
	if err != nil {
		return err
	}
	changed, _ := discoverChangedFiles(cfg.RepoPath)
	changed = indexableChangedFiles(changed, cfg)
	payload := map[string]any{
		"status":              status,
		"changed_files_count": len(changed),
		"changed_files":       changed,
	}
	if *jsonOut {
		return printJSON(stdout, payload)
	}
	fmt.Fprintf(stdout, "project: %s\n", status.ProjectID)
	fmt.Fprintf(stdout, "config: %s\n", status.ConfigPath)
	fmt.Fprintf(stdout, "repo: %s\n", status.RepoPath)
	fmt.Fprintf(stdout, "last index: %s %s %s\n", status.CommitSHA, status.Branch, status.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(stdout, "graph: %d files, %d symbols, %d relations\n", status.FilesCount, status.SymbolsCount, status.RelationsCount)
	fmt.Fprintf(stdout, "changed files: %d\n", len(changed))
	return nil
}

func runMCP(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	configPath := fs.String("config", ".seshat/project.yaml", "Path to project config")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, _, err := loadConfigWithHash(*configPath)
	if err != nil {
		return err
	}
	provider := &reloadingQueryProvider{
		configPath: filepath.Clean(*configPath),
		projectID:  cfg.ProjectID,
	}
	return mcpserver.NewServerWithProvider(provider.Query).Serve(stdin, stdout)
}

type reloadingQueryProvider struct {
	configPath string
	projectID  string
	modTime    time.Time
	size       int64
	query      *localquery.Service
}

func (p *reloadingQueryProvider) Query() (*localquery.Service, error) {
	info, err := os.Stat(localindex.GraphPath(p.configPath))
	if err != nil {
		return nil, err
	}
	if p.query != nil && info.Size() == p.size && info.ModTime().Equal(p.modTime) {
		return p.query, nil
	}
	batch, err := localindex.ReadGraph(p.configPath)
	if err != nil {
		return nil, err
	}
	query, err := localquery.New(p.projectID, batch)
	if err != nil {
		return nil, err
	}
	p.query = query
	p.modTime = info.ModTime()
	p.size = info.Size()
	return p.query, nil
}

func runGraph(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("graph", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", ".seshat/project.yaml", "Path to project config")
	filePath := fs.String("file", "", "Project-relative file path to graph")
	depth := fs.Int("depth", localquery.DefaultDepth, "Dependency traversal depth")
	format := fs.String("format", string(graphrender.FormatMermaid), "Output format: mermaid, dot, json")
	direction := fs.String("direction", string(graphrender.DirectionBoth), "Direction: both, depends-on, dependents")
	maxNodes := fs.Int("max-nodes", 25, "Maximum chart nodes")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *filePath == "" {
		return errors.New("usage: seshat graph --file cli/cmd/seshat/main.go --format mermaid")
	}
	cfg, _, err := loadConfigWithHash(*configPath)
	if err != nil {
		return err
	}
	batch, err := localindex.ReadGraph(*configPath)
	if err != nil {
		return err
	}
	query, err := localquery.New(cfg.ProjectID, batch)
	if err != nil {
		return err
	}
	graph, ok, err := query.FileDependencyGraph(cfg.ProjectID, filepath.Clean(*filePath), *depth)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("file %s not found in local index; run `seshat ingest` first if the index is stale", *filePath)
	}
	output, err := graphrender.Render(graph, graphrender.Options{
		Format:    graphrender.Format(*format),
		Direction: graphrender.Direction(*direction),
		MaxNodes:  *maxNodes,
	})
	if err != nil {
		return err
	}
	fmt.Fprint(stdout, output)
	return nil
}

func runSetup(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	configPath := fs.String("config", ".seshat/project.yaml", "Path to project config")
	client := fs.String("client", "all", "Client to configure: cursor, codex, claude, all")
	printOnly := fs.Bool("print", true, "Print config snippets")
	binaryPath := fs.String("binary", "seshat", "Seshat CLI command or absolute binary path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	setupConfigPath := *configPath
	if abs, absErr := filepath.Abs(*configPath); absErr == nil {
		setupConfigPath = abs
	}
	var projectID string
	if cfg, _, err := loadConfigWithHash(*configPath); err == nil {
		projectID = cfg.ProjectID
	}
	snippets, err := setup.Generate(setup.Client(*client), *binaryPath, setupConfigPath, projectID)
	if err != nil {
		return err
	}
	for _, snippet := range snippets {
		fmt.Fprintf(stdout, "# %s -> %s\n%s\n", snippet.Client, snippet.Path, snippet.Content)
	}
	if !*printOnly {
		return errors.New("file-writing setup is intentionally not enabled in v1; use --print and merge the snippet manually")
	}
	return nil
}

type buildOptions struct {
	Parallelism int
	Verbose     bool
	Log         io.Writer
}

func buildBatch(configPath, mode string, targetFiles []string) (model.AnalysisBatch, config.CLIProjectConfig, string, error) {
	return buildBatchWithOptions(configPath, mode, targetFiles, buildOptions{})
}

func buildBatchWithOptions(configPath, mode string, targetFiles []string, opts buildOptions) (model.AnalysisBatch, config.CLIProjectConfig, string, error) {
	cfg, configHash, err := loadConfigWithHash(configPath)
	if err != nil {
		return model.AnalysisBatch{}, config.CLIProjectConfig{}, "", err
	}
	analyzers := analyzerMap()
	if err := parser.ValidateTargets(cfg.LanguageTargets, analyzers); err != nil {
		return model.AnalysisBatch{}, config.CLIProjectConfig{}, "", err
	}
	commitSHA := gitValue(cfg.RepoPath, "rev-parse", "HEAD")
	branch := gitValue(cfg.RepoPath, "rev-parse", "--abbrev-ref", "HEAD")
	parallelism := opts.Parallelism
	if parallelism <= 0 {
		parallelism = 1
	}
	if opts.Verbose && opts.Log != nil {
		fmt.Fprintf(opts.Log, "ingest: repo=%s\n", cfg.RepoPath)
		fmt.Fprintf(opts.Log, "ingest: targets=%s mode=%s commit=%s branch=%s parallel=%d\n", strings.Join(cfg.LanguageTargets, ","), mode, commitSHA, branch, parallelism)
		if len(targetFiles) > 0 {
			fmt.Fprintf(opts.Log, "ingest: changed files=%d\n", len(targetFiles))
		}
	}
	input := parser.Input{
		ProjectID:     cfg.ProjectID,
		RepoPath:      cfg.RepoPath,
		IncludePaths:  cfg.IncludePaths,
		ExcludePaths:  cfg.ExcludePaths,
		TargetFiles:   targetFiles,
		CommitSHA:     commitSHA,
		Branch:        branch,
		SchemaVersion: graphschema.Version,
		ScanMode:      mode,
		Parallelism:   parallelism,
	}
	var batches []model.AnalysisBatch
	for _, target := range cfg.LanguageTargets {
		start := time.Now()
		if opts.Verbose && opts.Log != nil {
			fmt.Fprintf(opts.Log, "ingest: analyzing target=%s\n", target)
		}
		batch, err := analyzers[target].Analyze(context.Background(), input)
		if err != nil {
			if opts.Log != nil {
				fmt.Fprintf(opts.Log, "ingest: target=%s failed: %v\n", target, err)
			}
			return model.AnalysisBatch{}, config.CLIProjectConfig{}, "", err
		}
		if opts.Verbose && opts.Log != nil {
			summary := localindex.Summarize(batch)
			fmt.Fprintf(opts.Log, "ingest: target=%s done in %s (%d files, %d symbols, %d relations)\n", target, time.Since(start).Round(time.Millisecond), summary.FilesCount, summary.SymbolsCount, summary.RelationsCount)
		}
		batches = append(batches, batch)
	}
	if opts.Verbose && opts.Log != nil {
		fmt.Fprintf(opts.Log, "ingest: merging %d analysis batches\n", len(batches))
	}
	return parser.MergeBatches(cfg.ProjectID, commitSHA, branch, graphschema.Version, mode, batches...), cfg, configHash, nil
}

func loadConfigWithHash(configPath string) (config.CLIProjectConfig, string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return config.CLIProjectConfig{}, "", fmt.Errorf("read project config: %w", err)
	}
	cfg, err := config.LoadCLIProject(configPath)
	if err != nil {
		return config.CLIProjectConfig{}, "", err
	}
	if cfg.ProjectID == "" {
		return config.CLIProjectConfig{}, "", fmt.Errorf("project_id is required in %s", configPath)
	}
	cfg.RepoPath = filepath.Clean(cfg.RepoPath)
	return cfg, localindex.ConfigHash(data), nil
}

func analyzerMap() map[string]parser.Analyzer {
	return map[string]parser.Analyzer{
		"go":   goanalyzer.New(),
		"ruby": rubyanalyzer.New(),
	}
}

func detectProject(repoPath string) ([]string, []string, error) {
	languages := make(map[string]struct{})
	includeSet := make(map[string]struct{})
	err := filepath.WalkDir(repoPath, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(repoPath, path)
		if relErr != nil {
			return relErr
		}
		if entry.IsDir() {
			if watch.ShouldIgnore(rel, nil) {
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(rel) {
		case ".go":
			languages["go"] = struct{}{}
		case ".rb":
			languages["ruby"] = struct{}{}
		}
		first := strings.Split(filepath.ToSlash(rel), "/")[0]
		if first != "." && first != "" {
			includeSet[first] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	detected := make([]string, 0, len(languages))
	for language := range languages {
		detected = append(detected, language)
	}
	sort.Strings(detected)
	includes := make([]string, 0, len(includeSet))
	preferred := map[string]struct{}{"cmd": {}, "internal": {}, "pkg": {}, "app": {}, "lib": {}, "src": {}}
	for include := range includeSet {
		if _, ok := preferred[include]; ok {
			includes = append(includes, include)
		}
	}
	sort.Strings(includes)
	return detected, includes, nil
}

func projectIDFromPath(path string) string {
	base := strings.ToLower(filepath.Base(filepath.Clean(path)))
	var builder strings.Builder
	lastDash := false
	for _, r := range base {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	id := strings.Trim(builder.String(), "-")
	if id == "" {
		return "seshat-project"
	}
	return id
}

func mergeIncremental(current, delta model.AnalysisBatch, changed []string) model.AnalysisBatch {
	changedSet := make(map[string]struct{}, len(changed))
	for _, file := range changed {
		changedSet[filepath.Clean(file)] = struct{}{}
	}
	replacedFileIDs := make(map[string]struct{})
	for _, file := range current.Files {
		if _, ok := changedSet[filepath.Clean(file.Path)]; ok {
			replacedFileIDs[file.ID] = struct{}{}
		}
	}
	replacedSymbolIDs := make(map[string]struct{})
	for _, symbol := range current.Symbols {
		if _, ok := replacedFileIDs[symbol.FileID]; ok {
			replacedSymbolIDs[symbol.ID] = struct{}{}
		}
	}
	merged := model.AnalysisBatch{Metadata: delta.Metadata}
	for _, file := range current.Files {
		if _, ok := replacedFileIDs[file.ID]; !ok {
			merged.Files = append(merged.Files, file)
		}
	}
	for _, symbol := range current.Symbols {
		if _, ok := replacedSymbolIDs[symbol.ID]; !ok {
			merged.Symbols = append(merged.Symbols, symbol)
		}
	}
	for _, relation := range current.Relations {
		_, fromReplaced := replacedSymbolIDs[relation.FromSymbolID]
		_, toReplaced := replacedSymbolIDs[relation.ToSymbolID]
		if !fromReplaced && !toReplaced {
			merged.Relations = append(merged.Relations, relation)
		}
	}
	merged.Files = append(merged.Files, delta.Files...)
	merged.Symbols = append(merged.Symbols, delta.Symbols...)
	merged.Relations = append(merged.Relations, delta.Relations...)
	sort.Slice(merged.Files, func(i, j int) bool { return merged.Files[i].Path < merged.Files[j].Path })
	sort.Slice(merged.Symbols, func(i, j int) bool { return merged.Symbols[i].ID < merged.Symbols[j].ID })
	sort.Slice(merged.Relations, func(i, j int) bool { return merged.Relations[i].ID < merged.Relations[j].ID })
	return merged
}

func indexableChangedFiles(files []string, cfg config.CLIProjectConfig) []string {
	allowedExt := make(map[string]struct{})
	for _, target := range cfg.LanguageTargets {
		switch target {
		case "go":
			allowedExt[".go"] = struct{}{}
		case "ruby":
			allowedExt[".rb"] = struct{}{}
		}
	}
	out := make([]string, 0, len(files))
	for _, file := range files {
		rel := filepath.Clean(file)
		if watch.ShouldIgnore(rel, cfg.ExcludePaths) {
			continue
		}
		if !inIncludePath(rel, cfg.IncludePaths) {
			continue
		}
		if _, ok := allowedExt[filepath.Ext(rel)]; !ok {
			continue
		}
		out = append(out, rel)
	}
	sort.Strings(out)
	return out
}

func inIncludePath(rel string, includePaths []string) bool {
	if len(includePaths) == 0 {
		return true
	}
	clean := strings.Trim(filepath.Clean(rel), "/")
	for _, include := range includePaths {
		normalized := strings.Trim(filepath.Clean(include), "/")
		if normalized == "." || normalized == "" {
			continue
		}
		if clean == normalized || strings.HasPrefix(clean, normalized+"/") {
			return true
		}
	}
	return false
}

func printSummary(stdout io.Writer, action string, summary localindex.Summary, dryRun bool) {
	suffix := ""
	if dryRun {
		suffix = " (dry-run)"
	}
	fmt.Fprintf(stdout, "%s%s: %d files, %d symbols, %d relations\n", action, suffix, summary.FilesCount, summary.SymbolsCount, summary.RelationsCount)
	fmt.Fprintf(stdout, "project: %s\n", summary.ProjectID)
	fmt.Fprintf(stdout, "version: %s %s %s\n", summary.CommitSHA, summary.Branch, summary.GeneratedAt.Format(time.RFC3339))
	if len(summary.LanguageCounts) > 0 {
		fmt.Fprint(stdout, "languages:")
		for _, language := range localindex.SortedLanguages(summary.LanguageCounts) {
			fmt.Fprintf(stdout, " %s=%d", language, summary.LanguageCounts[language])
		}
		fmt.Fprintln(stdout)
	}
}

func printJSON(stdout io.Writer, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, string(data))
	return err
}

func gitValue(repo string, args ...string) string {
	cmd := execCommand("git", append([]string{"-C", filepath.Clean(repo)}, args...)...)
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

func usage(out io.Writer) {
	fmt.Fprintln(out, "Usage:")
	fmt.Fprintln(out, "  seshat init [--config .seshat/project.yaml]")
	fmt.Fprintln(out, "  seshat ingest [--config .seshat/project.yaml] [--parallel 1] [-v] [--dry-run] [--json]")
	fmt.Fprintln(out, "  seshat push [--config .seshat/project.yaml] [--force]")
	fmt.Fprintln(out, "  seshat watch [--config .seshat/project.yaml] [--debounce 2000]")
	fmt.Fprintln(out, "  seshat inspect [--config .seshat/project.yaml] [--json]")
	fmt.Fprintln(out, "  seshat status [--config .seshat/project.yaml] [--json]")
	fmt.Fprintln(out, "  seshat mcp [--config .seshat/project.yaml]")
	fmt.Fprintln(out, "  seshat graph --file path/to/file.go [--format mermaid|dot|json]")
	fmt.Fprintln(out, "  seshat setup [--client cursor|codex|claude|all] [--print]")
}

func discoverChangedFiles(repoPath string) ([]string, error) {
	if _, err := execCommand("git", "-C", filepath.Clean(repoPath), "rev-parse", "--is-inside-work-tree").Output(); err != nil {
		return nil, fmt.Errorf("discover changed files: %w", err)
	}

	candidates := make(map[string]struct{})
	diffCmd := execCommand("git", "-C", filepath.Clean(repoPath), "diff", "--name-only", "--diff-filter=ACMRTUXB", "HEAD")
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

	untrackedCmd := execCommand("git", "-C", filepath.Clean(repoPath), "ls-files", "--others", "--exclude-standard")
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

// Legacy server-backed commands are kept for old scripts, but CLI-first flows do not use them.
func runAuth(args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] != "verify" {
		return errors.New("usage: seshat auth verify --config .seshat/project.yaml")
	}
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
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
	fmt.Fprintln(stdout, string(body))
	return nil
}

func runDependencies(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("dependencies", flag.ContinueOnError)
	configPath := fs.String("config", ".seshat/project.yaml", "Path to project config")
	filePath := fs.String("file", "", "Project-relative file path to inspect")
	depth := fs.Int("depth", 1, "Dependency traversal depth")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *filePath == "" {
		return errors.New("usage: seshat dependencies --file internal/order/service.go --config .seshat/project.yaml")
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
	fmt.Fprintln(stdout, string(body))
	return nil
}

func runScan(args []string) error {
	return runIngest(args, os.Stdout, os.Stderr)
}
