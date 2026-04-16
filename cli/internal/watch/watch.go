package watch

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Snapshot map[string]time.Time

func SnapshotFiles(root string, includePaths, excludePaths []string) (Snapshot, error) {
	snapshot := make(Snapshot)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if entry.IsDir() {
			if rel != "." && ShouldIgnore(rel, excludePaths) {
				return filepath.SkipDir
			}
			return nil
		}
		if !included(rel, includePaths) || ShouldIgnore(rel, excludePaths) {
			return nil
		}
		ext := filepath.Ext(rel)
		if ext != ".go" && ext != ".rb" {
			return nil
		}
		info, statErr := entry.Info()
		if statErr != nil {
			return statErr
		}
		snapshot[filepath.Clean(rel)] = info.ModTime()
		return nil
	})
	return snapshot, err
}

func Changed(before, after Snapshot) []string {
	changed := make([]string, 0)
	for path, mod := range after {
		if previous, ok := before[path]; !ok || !previous.Equal(mod) {
			changed = append(changed, path)
		}
	}
	for path := range before {
		if _, ok := after[path]; !ok {
			changed = append(changed, path)
		}
	}
	sort.Strings(changed)
	return changed
}

func ShouldIgnore(rel string, extra []string) bool {
	clean := strings.Trim(filepath.Clean(rel), "/")
	if clean == "." || clean == "" {
		return false
	}
	defaults := []string{".git", ".seshat/index", "vendor", "node_modules", "dist", "build", "tmp", "coverage", ".cache"}
	for _, prefix := range append(defaults, extra...) {
		normalized := strings.Trim(filepath.Clean(prefix), "/")
		if normalized == "." || normalized == "" {
			continue
		}
		if clean == normalized || strings.HasPrefix(clean, normalized+"/") {
			return true
		}
	}
	return false
}

func included(rel string, includePaths []string) bool {
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
