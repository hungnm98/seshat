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

func Generate(client Client, binaryPath, registryPath string) ([]Snippet, error) {
	if binaryPath == "" {
		binaryPath = "seshat"
	}
	if registryPath == "" {
		registryPath = filepath.Join(".seshat", "config.yml")
	}
	mcpName := "seshat"
	switch client {
	case ClientCursor:
		return []Snippet{cursorSnippet(mcpName, binaryPath, registryPath)}, nil
	case ClientCodex:
		return []Snippet{codexSnippet(mcpName, binaryPath, registryPath)}, nil
	case ClientClaude:
		return claudeSnippets(mcpName, binaryPath, registryPath), nil
	case ClientAll:
		snippets := []Snippet{
			cursorSnippet(mcpName, binaryPath, registryPath),
			codexSnippet(mcpName, binaryPath, registryPath),
		}
		snippets = append(snippets, claudeSnippets(mcpName, binaryPath, registryPath)...)
		return snippets, nil
	default:
		return nil, fmt.Errorf("unsupported client %q", client)
	}
}

func cursorSnippet(mcpName, binaryPath, registryPath string) Snippet {
	args := mcpArgs(registryPath)
	payload := map[string]any{
		"mcpServers": map[string]any{
			mcpName: map[string]any{
				"command": binaryPath,
				"args":    args,
			},
		},
	}
	data, _ := json.MarshalIndent(payload, "", "  ")
	return Snippet{Client: ClientCursor, Path: ".cursor/mcp.json", Content: string(data)}
}

func codexSnippet(mcpName, binaryPath, registryPath string) Snippet {
	content := fmt.Sprintf(`[mcp_servers.%s]
command = %q
args = %s
`, mcpName, binaryPath, argsTOML(mcpArgs(registryPath)))
	return Snippet{Client: ClientCodex, Path: "$CODEX_HOME/config.toml", Content: content}
}

func claudeSnippets(mcpName, binaryPath, registryPath string) []Snippet {
	args := mcpArgs(registryPath)
	mcpPayload := map[string]any{
		"mcpServers": map[string]any{
			mcpName: map[string]any{
				"command": binaryPath,
				"args":    args,
				"type":    "stdio",
			},
		},
	}
	data, _ := json.MarshalIndent(mcpPayload, "", "  ")

	claudeMD := fmt.Sprintf("## Seshat MCP\nUse the `%s` MCP server for codebase analysis. Call `list_projects` to discover available `project_id` values. Every code query must pass `project_id`.\nThe shared MCP registry lives at `$HOME/.seshat/config.yml` and should be served with `seshat mcp --registry $HOME/.seshat/config.yml`.", mcpName)

	return []Snippet{
		{Client: ClientClaude, Path: ".claude/settings.json", Content: string(data)},
		{Client: ClientClaude, Path: "CLAUDE.md", Content: claudeMD},
	}
}

func mcpArgs(registryPath string) []string {
	return []string{"mcp", "--registry", filepath.Clean(registryPath)}
}

func argsTOML(args []string) string {
	data, _ := json.Marshal(args)
	return string(data)
}
