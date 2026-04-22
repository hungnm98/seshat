package watch

import (
	"path/filepath"
	"strings"
)

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
