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
	"sync"
	"time"

	"github.com/hungnm98/seshat-cli/internal/config"
	"github.com/hungnm98/seshat-cli/internal/graphrender"
	"github.com/hungnm98/seshat-cli/internal/localindex"
	"github.com/hungnm98/seshat-cli/internal/localquery"
	mcpserver "github.com/hungnm98/seshat-cli/internal/mcp"
	"github.com/hungnm98/seshat-cli/internal/parser"
	goanalyzer "github.com/hungnm98/seshat-cli/internal/parser/golang"
	jsanalyzer "github.com/hungnm98/seshat-cli/internal/parser/javascript"
	rubyanalyzer "github.com/hungnm98/seshat-cli/internal/parser/ruby"
	"github.com/hungnm98/seshat-cli/internal/setup"
	"github.com/hungnm98/seshat-cli/internal/version"
	"github.com/hungnm98/seshat-cli/internal/watch"
	"github.com/hungnm98/seshat-cli/pkg/graphschema"
	"github.com/hungnm98/seshat-cli/pkg/model"
	"gopkg.in/yaml.v3"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		log.Fatal(err)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) < 1 {
		usage(stdout)
		return nil
	}
	switch args[0] {
	case "help", "-h", "--help":
		usage(stdout)
		return nil
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "scan", "c":
		return runScan(args[1:], stdout, stderr)
	case "ingest":
		fmt.Fprintln(stderr, "`ingest` is deprecated; use `scan` instead.")
		return runScan(args[1:], stdout, stderr)
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
	case "version", "--version", "-v":
		fmt.Fprintf(stdout, "seshat %s\n", version.Version)
		return nil
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
	normalizedArgs, positionalRepo, err := normalizeInitArgs(args)
	if err != nil {
		return err
	}
	if err := fs.Parse(normalizedArgs); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		return errors.New("usage: seshat init [path] [--config .seshat/project.yaml]")
	}
	repo := *repoPath
	if positionalRepo != "" {
		repo = positionalRepo
	} else if fs.NArg() == 1 {
		repo = fs.Arg(0)
	}
	if _, err := os.Stat(*configPath); err == nil && !*force {
		return fmt.Errorf("%s already exists; pass --force to overwrite", *configPath)
	}
	repoAbs, err := filepath.Abs(repo)
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

func normalizeInitArgs(args []string) ([]string, string, error) {
	normalized := make([]string, 0, len(args))
	positionalRepo := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--config" || arg == "--project-id" || arg == "--repo":
			if i+1 >= len(args) {
				return nil, "", fmt.Errorf("%s requires a value", arg)
			}
			normalized = append(normalized, arg, args[i+1])
			i++
		case strings.HasPrefix(arg, "--config=") || strings.HasPrefix(arg, "--project-id=") || strings.HasPrefix(arg, "--repo="):
			normalized = append(normalized, arg)
		case arg == "--force":
			normalized = append(normalized, arg)
		case strings.HasPrefix(arg, "-"):
			normalized = append(normalized, arg)
		case positionalRepo == "":
			positionalRepo = arg
		default:
			normalized = append(normalized, arg)
		}
	}
	return normalized, positionalRepo, nil
}

func runScan(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", ".seshat/project.yaml", "Path to project config")
	all := fs.Bool("all", false, "Enable all local analysis features currently supported")
	dryRun := fs.Bool("dry-run", false, "Parse and print output without writing local index")
	jsonOut := fs.Bool("json", false, "Print JSON output")
	parallelism := fs.Int("parallel", 1, "Number of files to parse concurrently")
	fs.IntVar(parallelism, "p", 1, "Number of files to parse concurrently (shorthand)")
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
	printSummary(stdout, "scan", summary, *dryRun)
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
	if len(args) > 0 {
		switch args[0] {
		case "add":
			return runMCPAdd(args[1:], stdout)
		case "reload":
			return runMCPReload(args[1:], stdout)
		case "project":
			return runMCPProject(args[1:], stdout)
		case "config":
			return runMCPConfig(args[1:], stdout)
		case "-h", "--help":
			mcpUsage(stdout)
			return nil
		}
	}
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	registryPath := fs.String("registry", defaultMCPRegistryPath(), "Path to multi-project MCP registry")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		mcpUsage(stdout)
		return fmt.Errorf("unknown mcp command or argument %q", fs.Arg(0))
	}
	provider := newRegistryQueryProvider(*registryPath)
	return mcpserver.NewServerWithProjectProviderAndLister(provider.Query, provider.ListProjects).Serve(stdin, stdout)
}

const (
	defaultMCPRegistryName = "config.yml"
	defaultCacheIdleTTL    = 30 * time.Minute
)

type mcpRegistryConfig struct {
	Cache    mcpCacheConfig `yaml:"cache"`
	Projects []mcpProject   `yaml:"projects"`
}

type mcpCacheConfig struct {
	IdleTTL string `yaml:"idle_ttl"`
}

type mcpProject struct {
	ID     string `yaml:"id"`
	Path   string `yaml:"path"`
	Config string `yaml:"config"`
}

func runMCPAdd(args []string, stdout io.Writer) error {
	registryPath, reload, rest, err := parseMCPAddArgs(args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return errors.New("usage: seshat mcp add {folder_project} [--registry ~/.seshat/config.yml] [--reload]")
	}
	projectDir, err := filepath.Abs(rest[0])
	if err != nil {
		return err
	}
	projectConfig := filepath.Join(projectDir, ".seshat", "project.yaml")
	cfg, _, err := loadConfigWithHash(projectConfig)
	if err != nil {
		return err
	}
	registry, err := loadMCPRegistry(registryPath)
	if err != nil {
		return err
	}
	project := mcpProject{ID: cfg.ProjectID, Path: filepath.Clean(projectDir), Config: filepath.Clean(projectConfig)}
	registry.Projects = upsertMCPProject(registry.Projects, project)
	if err := writeMCPRegistry(registryPath, registry); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "added project %s %s\n", project.ID, project.Path)
	if reload {
		fmt.Fprintln(stdout, lazyReloadMessage())
	}
	return nil
}

func runMCPReload(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("mcp reload", flag.ContinueOnError)
	registryPath := fs.String("registry", defaultMCPRegistryPath(), "Path to multi-project MCP registry")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if _, err := loadMCPRegistry(*registryPath); err != nil {
		return err
	}
	fmt.Fprintln(stdout, lazyReloadMessage())
	return nil
}

func lazyReloadMessage() string {
	return "registry is valid; each stdio MCP process lazily reloads registry and graph files on its next tool call"
}

func runMCPProject(args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		mcpProjectUsage(stdout)
		return nil
	}
	switch args[0] {
	case "ls", "list":
		fs := flag.NewFlagSet("mcp project ls", flag.ContinueOnError)
		registryPath := fs.String("registry", defaultMCPRegistryPath(), "Path to multi-project MCP registry")
		jsonOut := fs.Bool("json", false, "Print JSON output")
		sortBy := fs.String("sort", "id", "Sort by id or path")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		registry, err := loadMCPRegistry(*registryPath)
		if err != nil {
			return err
		}
		sortMCPProjects(registry.Projects, *sortBy)
		if *jsonOut {
			return printJSON(stdout, registry.Projects)
		}
		for _, project := range registry.Projects {
			fmt.Fprintf(stdout, "%s\t%s\t%s\n", project.ID, project.Path, project.Config)
		}
		return nil
	default:
		mcpProjectUsage(stdout)
		return fmt.Errorf("unknown mcp project command %q", args[0])
	}
}

func runMCPConfig(args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		mcpConfigUsage(stdout)
		return nil
	}
	fs := flag.NewFlagSet("mcp config "+args[0], flag.ContinueOnError)
	registryPath := fs.String("registry", defaultMCPRegistryPath(), "Path to multi-project MCP registry")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	path := filepath.Clean(expandHome(*registryPath))
	switch args[0] {
	case "path":
		fmt.Fprintln(stdout, path)
		return nil
	case "show":
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fmt.Fprint(stdout, string(data))
		return nil
	case "edit":
		editor := os.Getenv("VISUAL")
		if editor == "" {
			editor = os.Getenv("EDITOR")
		}
		if editor == "" {
			return fmt.Errorf("VISUAL or EDITOR is required to edit %s", path)
		}
		cmd := execCommand(editor, path)
		cmd.Stdin = os.Stdin
		cmd.Stdout = stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	default:
		mcpConfigUsage(stdout)
		return fmt.Errorf("unknown mcp config command %q", args[0])
	}
}

func parseMCPAddArgs(args []string) (string, bool, []string, error) {
	registryPath := defaultMCPRegistryPath()
	reload := false
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--reload":
			reload = true
		case arg == "--registry":
			if i+1 >= len(args) {
				return "", false, nil, errors.New("--registry requires a path")
			}
			i++
			registryPath = args[i]
		case strings.HasPrefix(arg, "--registry="):
			registryPath = strings.TrimPrefix(arg, "--registry=")
		default:
			rest = append(rest, arg)
		}
	}
	return registryPath, reload, rest, nil
}

func loadMCPRegistry(path string) (mcpRegistryConfig, error) {
	path = filepath.Clean(expandHome(path))
	registry := mcpRegistryConfig{Cache: mcpCacheConfig{IdleTTL: defaultCacheIdleTTL.String()}}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return registry, nil
		}
		return mcpRegistryConfig{}, fmt.Errorf("read mcp registry: %w", err)
	}
	if err := yaml.Unmarshal(data, &registry); err != nil {
		return mcpRegistryConfig{}, fmt.Errorf("parse mcp registry: %w", err)
	}
	if registry.Cache.IdleTTL == "" {
		registry.Cache.IdleTTL = defaultCacheIdleTTL.String()
	}
	if _, err := parseCacheIdleTTL(registry.Cache.IdleTTL); err != nil {
		return mcpRegistryConfig{}, err
	}
	seen := make(map[string]string, len(registry.Projects))
	for i := range registry.Projects {
		registry.Projects[i].Path = filepath.Clean(expandHome(registry.Projects[i].Path))
		registry.Projects[i].Config = filepath.Clean(expandHome(registry.Projects[i].Config))
		if registry.Projects[i].ID == "" {
			return mcpRegistryConfig{}, fmt.Errorf("project id is required in %s", path)
		}
		if registry.Projects[i].Config == "" {
			return mcpRegistryConfig{}, fmt.Errorf("project %s config is required in %s", registry.Projects[i].ID, path)
		}
		if previous, exists := seen[registry.Projects[i].ID]; exists {
			return mcpRegistryConfig{}, fmt.Errorf("duplicate project id %q in %s (%s and %s)", registry.Projects[i].ID, path, previous, registry.Projects[i].Config)
		}
		seen[registry.Projects[i].ID] = registry.Projects[i].Config
	}
	sortMCPProjects(registry.Projects, "id")
	return registry, nil
}

func writeMCPRegistry(path string, registry mcpRegistryConfig) error {
	path = filepath.Clean(expandHome(path))
	if registry.Cache.IdleTTL == "" {
		registry.Cache.IdleTTL = defaultCacheIdleTTL.String()
	}
	sortMCPProjects(registry.Projects, "id")
	data, err := yaml.Marshal(registry)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func parseCacheIdleTTL(raw string) (time.Duration, error) {
	ttl, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("cache.idle_ttl must be a Go duration such as 30m or 1h: %w", err)
	}
	if ttl < 0 {
		return 0, fmt.Errorf("cache.idle_ttl must be >= 0")
	}
	return ttl, nil
}

func upsertMCPProject(projects []mcpProject, project mcpProject) []mcpProject {
	out := append([]mcpProject(nil), projects...)
	for i, existing := range out {
		if existing.ID == project.ID {
			out[i] = project
			sortMCPProjects(out, "id")
			return out
		}
	}
	out = append(out, project)
	sortMCPProjects(out, "id")
	return out
}

func sortMCPProjects(projects []mcpProject, sortBy string) {
	sort.Slice(projects, func(i, j int) bool {
		switch sortBy {
		case "path":
			if projects[i].Path == projects[j].Path {
				return projects[i].ID < projects[j].ID
			}
			return projects[i].Path < projects[j].Path
		default:
			if projects[i].ID == projects[j].ID {
				return projects[i].Path < projects[j].Path
			}
			return projects[i].ID < projects[j].ID
		}
	})
}

func defaultMCPRegistryPath() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".seshat", defaultMCPRegistryName)
	}
	return filepath.Join(".seshat", defaultMCPRegistryName)
}

func expandHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

type registryQueryProvider struct {
	registryPath string
	mu           sync.Mutex
	modTime      time.Time
	size         int64
	idleTTL      time.Duration
	projects     []mcpProject
	providers    map[string]*reloadingQueryProvider
}

func newRegistryQueryProvider(registryPath string) *registryQueryProvider {
	return &registryQueryProvider{registryPath: filepath.Clean(expandHome(registryPath))}
}

func (p *registryQueryProvider) Query(projectID string) (*localquery.Service, error) {
	p.mu.Lock()
	if err := p.reloadIfChanged(); err != nil {
		p.mu.Unlock()
		return nil, err
	}
	p.evictIdle(time.Now())
	provider := p.providers[projectID]
	if provider == nil {
		p.mu.Unlock()
		return nil, fmt.Errorf("project %q is not registered; call list_projects for available project_id values", projectID)
	}
	p.mu.Unlock()
	return provider.Query()
}

func (p *registryQueryProvider) ListProjects() ([]mcpserver.ProjectInfo, error) {
	p.mu.Lock()
	if err := p.reloadIfChanged(); err != nil {
		p.mu.Unlock()
		return nil, err
	}
	registryProjects := append([]mcpProject(nil), p.projects...)
	p.mu.Unlock()

	projects := make([]mcpserver.ProjectInfo, 0, len(registryProjects))
	for _, project := range registryProjects {
		cfg, err := config.LoadCLIProject(project.Config)
		if err != nil {
			projects = append(projects, mcpserver.ProjectInfo{
				ProjectID: project.ID,
				Path:      project.Path,
				Config:    project.Config,
				Status:    "invalid",
				Hint:      err.Error(),
			})
			continue
		}
		if cfg.ProjectID == "" {
			cfg.ProjectID = project.ID
		}
		projects = append(projects, projectInfoFromConfig(project.Config, cfg))
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].ProjectID < projects[j].ProjectID })
	return projects, nil
}

func (p *registryQueryProvider) reloadIfChanged() error {
	info, err := os.Stat(p.registryPath)
	if err != nil {
		return err
	}
	if p.providers != nil && info.Size() == p.size && info.ModTime().Equal(p.modTime) {
		return nil
	}
	registry, err := loadMCPRegistry(p.registryPath)
	if err != nil {
		return err
	}
	idleTTL, err := parseCacheIdleTTL(registry.Cache.IdleTTL)
	if err != nil {
		return err
	}
	providers := make(map[string]*reloadingQueryProvider, len(registry.Projects))
	for _, project := range registry.Projects {
		existing := p.providers[project.ID]
		if existing != nil && existing.configPath == project.Config {
			providers[project.ID] = existing
			continue
		}
		providers[project.ID] = &reloadingQueryProvider{configPath: project.Config, projectID: project.ID}
	}
	p.projects = registry.Projects
	p.providers = providers
	p.idleTTL = idleTTL
	p.modTime = info.ModTime()
	p.size = info.Size()
	return nil
}

func (p *registryQueryProvider) evictIdle(now time.Time) {
	if p.idleTTL == 0 {
		return
	}
	for _, provider := range p.providers {
		provider.EvictIdle(now, p.idleTTL)
	}
}

func projectInfoFromConfig(configPath string, cfg config.CLIProjectConfig) mcpserver.ProjectInfo {
	info := mcpserver.ProjectInfo{
		ProjectID: cfg.ProjectID,
		Path:      filepath.Clean(cfg.RepoPath),
		Config:    filepath.Clean(configPath),
		Status:    "not_indexed",
		Hint:      "run seshat scan",
	}
	status, err := localindex.ReadStatus(configPath)
	if err != nil {
		return info
	}
	info.Status = "indexed"
	info.Hint = ""
	info.LastIndexedAt = status.GeneratedAt.Format(time.RFC3339)
	info.FilesCount = status.FilesCount
	info.SymbolsCount = status.SymbolsCount
	info.RelationsCount = status.RelationsCount
	return info
}

type reloadingQueryProvider struct {
	mu         sync.Mutex
	configPath string
	projectID  string
	modTime    time.Time
	size       int64
	lastUsed   time.Time
	query      *localquery.Service
}

func (p *reloadingQueryProvider) Query() (*localquery.Service, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	info, err := os.Stat(localindex.GraphPath(p.configPath))
	if err != nil {
		return nil, err
	}
	if p.query != nil && info.Size() == p.size && info.ModTime().Equal(p.modTime) {
		p.lastUsed = now
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
	p.lastUsed = now
	return p.query, nil
}

func (p *reloadingQueryProvider) EvictIdle(now time.Time, ttl time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.query == nil || p.lastUsed.IsZero() || now.Sub(p.lastUsed) < ttl {
		return
	}
	p.query = nil
	p.modTime = time.Time{}
	p.size = 0
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
		return fmt.Errorf("file %s not found in local index; run `seshat scan` first if the index is stale", *filePath)
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
	registryPath := fs.String("registry", defaultMCPRegistryPath(), "Path to multi-project MCP registry")
	client := fs.String("client", "all", "Client to configure: cursor, codex, claude, all")
	printOnly := fs.Bool("print", true, "Print config snippets")
	binaryPath := fs.String("binary", "seshat", "Seshat CLI command or absolute binary path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	setupConfigPath := filepath.Clean(expandHome(*registryPath))
	if abs, absErr := filepath.Abs(setupConfigPath); absErr == nil {
		setupConfigPath = abs
	}
	snippets, err := setup.Generate(setup.Client(*client), *binaryPath, setupConfigPath)
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
		"go":         goanalyzer.New(),
		"ruby":       rubyanalyzer.New(),
		"javascript": jsanalyzer.NewJS(),
		"typescript": jsanalyzer.NewTS(),
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
		case ".js", ".jsx":
			languages["javascript"] = struct{}{}
		case ".ts", ".tsx":
			languages["typescript"] = struct{}{}
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

func indexableChangedFiles(files []string, cfg config.CLIProjectConfig) []string {
	allowedExt := make(map[string]struct{})
	for _, target := range cfg.LanguageTargets {
		switch target {
		case "go":
			allowedExt[".go"] = struct{}{}
		case "ruby":
			allowedExt[".rb"] = struct{}{}
		case "javascript":
			allowedExt[".js"] = struct{}{}
			allowedExt[".jsx"] = struct{}{}
		case "typescript":
			allowedExt[".ts"] = struct{}{}
			allowedExt[".tsx"] = struct{}{}
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
	fmt.Fprintln(out, "  seshat init [path] [--config .seshat/project.yaml]")
	fmt.Fprintln(out, "  seshat scan|c [--config .seshat/project.yaml] [--parallel|-p 1] [-v] [--dry-run] [--json]")
	fmt.Fprintln(out, "  seshat inspect [--config .seshat/project.yaml] [--json]")
	fmt.Fprintln(out, "  seshat status [--config .seshat/project.yaml] [--json]")
	fmt.Fprintln(out, "  seshat mcp [--registry ~/.seshat/config.yml]")
	fmt.Fprintln(out, "  seshat graph --file path/to/file.go [--format mermaid|dot|json]")
	fmt.Fprintln(out, "  seshat setup [--registry ~/.seshat/config.yml] [--client cursor|codex|claude|all] [--print]")
	fmt.Fprintln(out, "  seshat version")
}

func mcpUsage(out io.Writer) {
	fmt.Fprintln(out, "Usage:")
	fmt.Fprintln(out, "  seshat mcp [--registry ~/.seshat/config.yml]")
	fmt.Fprintln(out, "  seshat mcp add {folder_project} [--registry ~/.seshat/config.yml] [--reload]")
	fmt.Fprintln(out, "  seshat mcp reload [--registry ~/.seshat/config.yml]")
	fmt.Fprintln(out, "  seshat mcp project ls [--registry ~/.seshat/config.yml] [--sort id|path] [--json]")
	fmt.Fprintln(out, "  seshat mcp config path|show|edit [--registry ~/.seshat/config.yml]")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Notes:")
	fmt.Fprintln(out, "  --registry serves one MCP for all registered projects over stdio.")
	fmt.Fprintln(out, "  MCP clients should call list_projects first when project_id is unknown.")
	fmt.Fprintln(out, "  reload validates config; stdio MCP processes lazily reload on their next tool call.")
}

func mcpProjectUsage(out io.Writer) {
	fmt.Fprintln(out, "Usage:")
	fmt.Fprintln(out, "  seshat mcp project ls [--registry ~/.seshat/config.yml] [--sort id|path] [--json]")
}

func mcpConfigUsage(out io.Writer) {
	fmt.Fprintln(out, "Usage:")
	fmt.Fprintln(out, "  seshat mcp config path [--registry ~/.seshat/config.yml]")
	fmt.Fprintln(out, "  seshat mcp config show [--registry ~/.seshat/config.yml]")
	fmt.Fprintln(out, "  seshat mcp config edit [--registry ~/.seshat/config.yml]")
}

func discoverChangedFiles(repoPath string) ([]string, error) {
	clean := filepath.Clean(repoPath)
	if _, err := execCommand("git", "-C", clean, "rev-parse", "--is-inside-work-tree").Output(); err != nil {
		return nil, fmt.Errorf("discover changed files: %w", err)
	}

	// git always outputs paths relative to the git root, but repoPath may be a
	// subdirectory of the root (e.g. a monorepo). Compute the prefix to strip.
	gitRootBytes, err := execCommand("git", "-C", clean, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return nil, fmt.Errorf("discover changed files git root: %w", err)
	}
	gitRoot := filepath.Clean(strings.TrimSpace(string(gitRootBytes)))
	repoAbs, err := filepath.Abs(clean)
	if err != nil {
		return nil, err
	}
	// Resolve symlinks so both sides use canonical paths (e.g. /tmp → /private/tmp on macOS).
	if r, err2 := filepath.EvalSymlinks(gitRoot); err2 == nil {
		gitRoot = r
	}
	if r, err2 := filepath.EvalSymlinks(repoAbs); err2 == nil {
		repoAbs = r
	}
	prefix, err := filepath.Rel(gitRoot, repoAbs)
	if err != nil {
		return nil, err
	}
	// stripPrefix converts a git-root-relative path to a repoPath-relative path.
	// Returns ("", false) when the path is outside repoPath.
	stripPrefix := func(p string) (string, bool) {
		if prefix == "." {
			return p, true
		}
		rel, relErr := filepath.Rel(prefix, p)
		if relErr != nil || strings.HasPrefix(rel, "..") {
			return "", false
		}
		return rel, true
	}

	candidates := make(map[string]struct{})
	diffCmd := execCommand("git", "-C", clean, "diff", "--name-only", "--diff-filter=ACMRTUXB", "HEAD")
	diffOutput, err := diffCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("discover changed files diff: %w", err)
	}
	for _, line := range strings.Split(string(diffOutput), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if rel, ok := stripPrefix(line); ok {
			candidates[rel] = struct{}{}
		}
	}

	untrackedCmd := execCommand("git", "-C", clean, "ls-files", "--others", "--exclude-standard")
	untrackedOutput, err := untrackedCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("discover changed files untracked: %w", err)
	}
	for _, line := range strings.Split(string(untrackedOutput), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if rel, ok := stripPrefix(line); ok {
			candidates[rel] = struct{}{}
		}
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
