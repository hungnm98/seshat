package setup

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

type Client string

const (
	ClientCursor Client = "cursor"
	ClientCodex  Client = "codex"
	ClientClaude Client = "claude"
	ClientAll    Client = "all"
)

type Snippet struct {
	Client  Client `json:"client"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

func Generate(client Client, binaryPath, configPath, projectID string) ([]Snippet, error) {
	if binaryPath == "" {
		binaryPath = "seshat"
	}
	if configPath == "" {
		configPath = ".seshat/project.yaml"
	}
	mcpName := "seshat"
	if projectID != "" {
		mcpName = "seshat-" + projectID
	}
	switch client {
	case ClientCursor:
		return []Snippet{cursorSnippet(mcpName, binaryPath, configPath)}, nil
	case ClientCodex:
		return []Snippet{codexSnippet(mcpName, binaryPath, configPath)}, nil
	case ClientClaude:
		return claudeSnippets(mcpName, binaryPath, configPath), nil
	case ClientAll:
		snippets := []Snippet{
			cursorSnippet(mcpName, binaryPath, configPath),
			codexSnippet(mcpName, binaryPath, configPath),
		}
		snippets = append(snippets, claudeSnippets(mcpName, binaryPath, configPath)...)
		return snippets, nil
	default:
		return nil, fmt.Errorf("unsupported client %q", client)
	}
}

func cursorSnippet(mcpName, binaryPath, configPath string) Snippet {
	payload := map[string]any{
		"mcpServers": map[string]any{
			mcpName: map[string]any{
				"command": binaryPath,
				"args":    []string{"mcp", "--config", filepath.Clean(configPath)},
			},
		},
	}
	data, _ := json.MarshalIndent(payload, "", "  ")
	return Snippet{Client: ClientCursor, Path: ".cursor/mcp.json", Content: string(data)}
}

func codexSnippet(mcpName, binaryPath, configPath string) Snippet {
	content := fmt.Sprintf(`[mcp_servers.%s]
command = %q
args = ["mcp", "--config", %q]
`, mcpName, binaryPath, filepath.Clean(configPath))
	return Snippet{Client: ClientCodex, Path: "$CODEX_HOME/config.toml", Content: content}
}

func claudeSnippets(mcpName, binaryPath, configPath string) []Snippet {
	mcpPayload := map[string]any{
		"mcpServers": map[string]any{
			mcpName: map[string]any{
				"command": binaryPath,
				"args":    []string{"mcp", "--config", filepath.Clean(configPath)},
				"type":    "stdio",
			},
		},
	}
	data, _ := json.MarshalIndent(mcpPayload, "", "  ")

	claudeMD := fmt.Sprintf("## Seshat MCP\nThis project uses the `%s` MCP server for codebase analysis.\nWhen exploring symbols, files, or dependencies, always use `%s` tools (not other seshat MCP servers).", mcpName, mcpName)

	return []Snippet{
		{Client: ClientClaude, Path: ".claude/settings.json", Content: string(data)},
		{Client: ClientClaude, Path: "CLAUDE.md", Content: claudeMD},
	}
}
