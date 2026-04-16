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
	ClientAll    Client = "all"
)

type Snippet struct {
	Client  Client `json:"client"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

func Generate(client Client, binaryPath, configPath string) ([]Snippet, error) {
	if binaryPath == "" {
		binaryPath = "seshat"
	}
	if configPath == "" {
		configPath = ".seshat/project.yaml"
	}
	switch client {
	case ClientCursor:
		return []Snippet{cursorSnippet(binaryPath, configPath)}, nil
	case ClientCodex:
		return []Snippet{codexSnippet(binaryPath, configPath)}, nil
	case ClientAll:
		return []Snippet{cursorSnippet(binaryPath, configPath), codexSnippet(binaryPath, configPath)}, nil
	default:
		return nil, fmt.Errorf("unsupported client %q", client)
	}
}

func cursorSnippet(binaryPath, configPath string) Snippet {
	payload := map[string]any{
		"mcpServers": map[string]any{
			"seshat": map[string]any{
				"command": binaryPath,
				"args":    []string{"mcp", "--config", filepath.Clean(configPath)},
			},
		},
	}
	data, _ := json.MarshalIndent(payload, "", "  ")
	return Snippet{Client: ClientCursor, Path: ".cursor/mcp.json", Content: string(data)}
}

func codexSnippet(binaryPath, configPath string) Snippet {
	content := fmt.Sprintf(`[mcp_servers.seshat]
command = %q
args = ["mcp", "--config", %q]
`, binaryPath, filepath.Clean(configPath))
	return Snippet{Client: ClientCodex, Path: "$CODEX_HOME/config.toml", Content: content}
}
