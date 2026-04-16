package projectmgmt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

var allowedStatuses = map[string]struct{}{
	"todo":        {},
	"in_progress": {},
	"blocked":     {},
	"done":        {},
}

type TaskMeta struct {
	ID        string   `yaml:"id"`
	Title     string   `yaml:"title"`
	Status    string   `yaml:"status"`
	DependsOn []string `yaml:"depends_on"`
}

func ValidateTasks(root string) error {
	pattern := filepath.Join(root, "tasks", "items", "*.md")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}
	index := make(map[string]TaskMeta)
	for _, file := range files {
		meta, err := readTaskMeta(file)
		if err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
		if meta.ID == "" {
			return fmt.Errorf("%s: missing id", file)
		}
		if _, ok := allowedStatuses[meta.Status]; !ok {
			return fmt.Errorf("%s: invalid status %s", file, meta.Status)
		}
		index[meta.ID] = meta
	}
	for _, meta := range index {
		for _, dependency := range meta.DependsOn {
			if _, ok := index[dependency]; !ok {
				return fmt.Errorf("task %s depends on missing task %s", meta.ID, dependency)
			}
		}
	}
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var walk func(string) error
	walk = func(id string) error {
		if visiting[id] {
			return fmt.Errorf("dependency cycle at %s", id)
		}
		if visited[id] {
			return nil
		}
		visiting[id] = true
		for _, dep := range index[id].DependsOn {
			if err := walk(dep); err != nil {
				return err
			}
		}
		visiting[id] = false
		visited[id] = true
		return nil
	}
	for id := range index {
		if err := walk(id); err != nil {
			return err
		}
	}
	roadmapBytes, err := os.ReadFile(filepath.Join(root, "tasks", "boards", "roadmap.md"))
	if err != nil {
		return err
	}
	roadmap := string(roadmapBytes)
	for id := range index {
		if !strings.Contains(roadmap, id) {
			return fmt.Errorf("roadmap.md does not reference task %s", id)
		}
	}
	return nil
}

func readTaskMeta(path string) (TaskMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return TaskMeta{}, err
	}
	content := string(data)
	if !strings.HasPrefix(content, "---\n") {
		return TaskMeta{}, fmt.Errorf("missing yaml front matter")
	}
	parts := strings.SplitN(strings.TrimPrefix(content, "---\n"), "\n---\n", 2)
	if len(parts) != 2 {
		return TaskMeta{}, fmt.Errorf("invalid yaml front matter")
	}
	var meta TaskMeta
	if err := yaml.Unmarshal([]byte(parts[0]), &meta); err != nil {
		return TaskMeta{}, err
	}
	return meta, nil
}
