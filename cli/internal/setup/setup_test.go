package setup

import (
	"strings"
	"testing"
)

func TestGenerateAllSnippets(t *testing.T) {
	snippets, err := Generate(ClientAll, "/usr/local/bin/seshat", "/repo/.seshat/project.yaml", "myproject")
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	// cursor + codex + claude settings + claude md = 4
	if len(snippets) != 4 {
		t.Fatalf("expected 4 snippets, got %d", len(snippets))
	}
	for _, snippet := range snippets {
		if snippet.Path == "CLAUDE.md" {
			continue
		}
		if !strings.Contains(snippet.Content, "mcp") || !strings.Contains(snippet.Content, "/repo/.seshat/project.yaml") {
			t.Fatalf("unexpected snippet content: %#v", snippet)
		}
		if strings.Contains(strings.ToLower(snippet.Content), "token") {
			t.Fatalf("snippet should not contain token fields: %s", snippet.Content)
		}
	}
}

func TestMCPNameUsesProjectID(t *testing.T) {
	snippets, err := Generate(ClientCursor, "seshat", ".seshat/project.yaml", "dax-be")
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if !strings.Contains(snippets[0].Content, "seshat-dax-be") {
		t.Fatalf("expected MCP name seshat-dax-be in content: %s", snippets[0].Content)
	}
}

func TestMCPNameFallbackWithoutProjectID(t *testing.T) {
	snippets, err := Generate(ClientCursor, "seshat", ".seshat/project.yaml", "")
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if !strings.Contains(snippets[0].Content, `"seshat"`) {
		t.Fatalf("expected fallback MCP name seshat in content: %s", snippets[0].Content)
	}
}

func TestClaudeSnippetsContainMCPAndInstruction(t *testing.T) {
	snippets, err := Generate(ClientClaude, "seshat", ".seshat/project.yaml", "my-app")
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if len(snippets) != 2 {
		t.Fatalf("expected 2 claude snippets, got %d", len(snippets))
	}
	hasMCP, hasMD := false, false
	for _, s := range snippets {
		if s.Path == ".claude/settings.json" {
			hasMCP = true
			if !strings.Contains(s.Content, "seshat-my-app") {
				t.Fatalf("settings.json missing MCP name: %s", s.Content)
			}
		}
		if s.Path == "CLAUDE.md" {
			hasMD = true
			if !strings.Contains(s.Content, "seshat-my-app") {
				t.Fatalf("CLAUDE.md missing MCP name: %s", s.Content)
			}
		}
	}
	if !hasMCP || !hasMD {
		t.Fatalf("missing expected snippets: hasMCP=%v hasMD=%v", hasMCP, hasMD)
	}
}
