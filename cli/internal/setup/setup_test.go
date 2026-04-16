package setup

import (
	"strings"
	"testing"
)

func TestGenerateCursorAndCodexSnippets(t *testing.T) {
	snippets, err := Generate(ClientAll, "/usr/local/bin/seshat", "/repo/.seshat/project.yaml")
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if len(snippets) != 2 {
		t.Fatalf("expected two snippets, got %d", len(snippets))
	}
	for _, snippet := range snippets {
		if !strings.Contains(snippet.Content, "mcp") || !strings.Contains(snippet.Content, "/repo/.seshat/project.yaml") {
			t.Fatalf("unexpected snippet: %#v", snippet)
		}
		if strings.Contains(strings.ToLower(snippet.Content), "token") {
			t.Fatalf("snippet should not contain token fields: %s", snippet.Content)
		}
	}
}
