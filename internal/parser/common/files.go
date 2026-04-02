package common

import (
	"crypto/sha1"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func CollectFiles(root string, includePaths, excludePaths []string, extensions map[string]struct{}) ([]string, error) {
	includeSet := normalizePaths(includePaths)
	excludeSet := normalizePaths(excludePaths)
	var files []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			rel, relErr := filepath.Rel(root, path)
			if relErr == nil && rel != "." && shouldSkip(rel, excludeSet) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if !inInclude(rel, includeSet) || shouldSkip(rel, excludeSet) {
			return nil
		}
		if _, ok := extensions[filepath.Ext(rel)]; ok {
			files = append(files, rel)
		}
		return nil
	})
	return files, err
}

func FileChecksum(root, rel string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return "", err
	}
	sum := sha1.Sum(data)
	return hex.EncodeToString(sum[:]), nil
}

func normalizePaths(paths []string) map[string]struct{} {
	if len(paths) == 0 {
		return map[string]struct{}{}
	}
	set := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		trimmed := strings.Trim(filepath.Clean(path), "/")
		if trimmed == "." {
			continue
		}
		set[trimmed] = struct{}{}
	}
	return set
}

func inInclude(rel string, include map[string]struct{}) bool {
	if len(include) == 0 {
		return true
	}
	for prefix := range include {
		if rel == prefix || strings.HasPrefix(rel, prefix+"/") {
			return true
		}
	}
	return false
}

func shouldSkip(rel string, exclude map[string]struct{}) bool {
	for prefix := range exclude {
		if rel == prefix || strings.HasPrefix(rel, prefix+"/") {
			return true
		}
	}
	return false
}
