package localindex

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/hungnm98/seshat-cli/pkg/model"
)

const (
	GraphFile  = "graph.json"
	StatusFile = "status.json"
)

type Status struct {
	ProjectID      string         `json:"project_id"`
	ConfigPath     string         `json:"config_path"`
	ConfigHash     string         `json:"config_hash"`
	RepoPath       string         `json:"repo_path"`
	CommitSHA      string         `json:"commit_sha"`
	Branch         string         `json:"branch"`
	SchemaVersion  string         `json:"schema_version"`
	ScanMode       string         `json:"scan_mode"`
	GeneratedAt    time.Time      `json:"generated_at"`
	FilesCount     int            `json:"files_count"`
	SymbolsCount   int            `json:"symbols_count"`
	RelationsCount int            `json:"relations_count"`
	LanguageCounts map[string]int `json:"language_counts"`
}

type Summary struct {
	ProjectID      string         `json:"project_id"`
	CommitSHA      string         `json:"commit_sha"`
	Branch         string         `json:"branch"`
	SchemaVersion  string         `json:"schema_version"`
	GeneratedAt    time.Time      `json:"generated_at"`
	ScanMode       string         `json:"scan_mode"`
	FilesCount     int            `json:"files_count"`
	SymbolsCount   int            `json:"symbols_count"`
	RelationsCount int            `json:"relations_count"`
	LanguageCounts map[string]int `json:"language_counts"`
}

func IndexDir(configPath string) string {
	return filepath.Join(filepath.Dir(filepath.Clean(configPath)), "index")
}

func GraphPath(configPath string) string {
	return filepath.Join(IndexDir(configPath), GraphFile)
}

func StatusPath(configPath string) string {
	return filepath.Join(IndexDir(configPath), StatusFile)
}

func Ensure(configPath string) error {
	return os.MkdirAll(IndexDir(configPath), 0o755)
}

func WriteGraph(configPath string, batch model.AnalysisBatch) error {
	if err := Ensure(configPath); err != nil {
		return err
	}
	return writeJSON(GraphPath(configPath), batch)
}

func ReadGraph(configPath string) (model.AnalysisBatch, error) {
	data, err := os.ReadFile(GraphPath(configPath))
	if err != nil {
		if os.IsNotExist(err) {
			return model.AnalysisBatch{}, fmt.Errorf("local index not found at %s; run `seshat ingest` first", GraphPath(configPath))
		}
		return model.AnalysisBatch{}, fmt.Errorf("read graph index: %w", err)
	}
	var batch model.AnalysisBatch
	if err := json.Unmarshal(data, &batch); err != nil {
		return model.AnalysisBatch{}, fmt.Errorf("parse graph index: %w", err)
	}
	return batch, nil
}

func WriteStatus(configPath string, status Status) error {
	if err := Ensure(configPath); err != nil {
		return err
	}
	return writeJSON(StatusPath(configPath), status)
}

func ReadStatus(configPath string) (Status, error) {
	data, err := os.ReadFile(StatusPath(configPath))
	if err != nil {
		if os.IsNotExist(err) {
			return Status{}, fmt.Errorf("local status not found at %s; run `seshat ingest` first", StatusPath(configPath))
		}
		return Status{}, fmt.Errorf("read index status: %w", err)
	}
	var status Status
	if err := json.Unmarshal(data, &status); err != nil {
		return Status{}, fmt.Errorf("parse index status: %w", err)
	}
	return status, nil
}

func BuildStatus(configPath, repoPath, configHash string, batch model.AnalysisBatch) Status {
	summary := Summarize(batch)
	return Status{
		ProjectID:      summary.ProjectID,
		ConfigPath:     filepath.Clean(configPath),
		ConfigHash:     configHash,
		RepoPath:       filepath.Clean(repoPath),
		CommitSHA:      summary.CommitSHA,
		Branch:         summary.Branch,
		SchemaVersion:  summary.SchemaVersion,
		ScanMode:       summary.ScanMode,
		GeneratedAt:    summary.GeneratedAt,
		FilesCount:     summary.FilesCount,
		SymbolsCount:   summary.SymbolsCount,
		RelationsCount: summary.RelationsCount,
		LanguageCounts: summary.LanguageCounts,
	}
}

func Summarize(batch model.AnalysisBatch) Summary {
	languages := make(map[string]int)
	for _, file := range batch.Files {
		languages[file.Language]++
	}
	return Summary{
		ProjectID:      batch.Metadata.ProjectID,
		CommitSHA:      batch.Metadata.CommitSHA,
		Branch:         batch.Metadata.Branch,
		SchemaVersion:  batch.Metadata.SchemaVersion,
		GeneratedAt:    batch.Metadata.GeneratedAt,
		ScanMode:       batch.Metadata.ScanMode,
		FilesCount:     len(batch.Files),
		SymbolsCount:   len(batch.Symbols),
		RelationsCount: len(batch.Relations),
		LanguageCounts: languages,
	}
}

func ConfigHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func SortedLanguages(counts map[string]int) []string {
	languages := make([]string, 0, len(counts))
	for language := range counts {
		languages = append(languages, language)
	}
	sort.Strings(languages)
	return languages
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
