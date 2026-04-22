package setup

import (
	"strings"
	"testing"
)

func TestGenerateAllSnippets(t *testing.T) {
	snippets, err := Generate(ClientAll, "/usr/local/bin/seshat", "/home/me/.seshat/config.yml")
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
		if !strings.Contains(snippet.Content, "mcp") || !strings.Contains(snippet.Content, "/home/me/.seshat/config.yml") {
			t.Fatalf("unexpected snippet content: %#v", snippet)
		}
		if strings.Contains(strings.ToLower(snippet.Content), "token") {
			t.Fatalf("snippet should not contain token fields: %s", snippet.Content)
		}
	}
}

func TestMCPNameIsSharedSeshat(t *testing.T) {
	snippets, err := Generate(ClientCursor, "seshat", "/home/me/.seshat/config.yml")
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if !strings.Contains(snippets[0].Content, `"seshat"`) || strings.Contains(snippets[0].Content, "seshat-") {
		t.Fatalf("expected shared MCP name seshat in content: %s", snippets[0].Content)
	}
}

func TestSnippetsUseRegistryArg(t *testing.T) {
	snippets, err := Generate(ClientCursor, "seshat", "/home/me/.seshat/config.yml")
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if !strings.Contains(snippets[0].Content, "--registry") || strings.Contains(snippets[0].Content, "--config") {
		t.Fatalf("expected registry MCP args in content: %s", snippets[0].Content)
	}
}

func TestClaudeSnippetsContainMCPAndInstruction(t *testing.T) {
	snippets, err := Generate(ClientClaude, "seshat", "/home/me/.seshat/config.yml")
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
			if !strings.Contains(s.Content, `"seshat"`) || strings.Contains(s.Content, "seshat-") {
				t.Fatalf("settings.json missing MCP name: %s", s.Content)
			}
		}
		if s.Path == "CLAUDE.md" {
			hasMD = true
			if !strings.Contains(s.Content, "`seshat` MCP server") {
				t.Fatalf("CLAUDE.md missing MCP name: %s", s.Content)
			}
		}
	}
	if !hasMCP || !hasMD {
		t.Fatalf("missing expected snippets: hasMCP=%v hasMD=%v", hasMCP, hasMD)
	}
}

func TestRegistrySnippetsUseSingleSeshatMCP(t *testing.T) {
	snippets, err := Generate(ClientAll, "seshat", "/home/me/.seshat/config.yml")
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	for _, snippet := range snippets {
		if !strings.Contains(snippet.Content, "seshat") {
			t.Fatalf("expected seshat MCP name in content: %#v", snippet)
		}
		if snippet.Path != "CLAUDE.md" && !strings.Contains(snippet.Content, "--registry") {
			t.Fatalf("expected registry arg in content: %#v", snippet)
		}
		if strings.Contains(snippet.Content, "seshat-ignored") {
			t.Fatalf("registry mode should not include project-specific MCP name: %s", snippet.Content)
		}
	}
}
