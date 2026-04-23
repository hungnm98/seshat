# Seshat

Seshat is a local code knowledge graph for AI coding agents. It scans local repositories, writes a lightweight graph index, and exposes all registered projects through one shared MCP server named `seshat`.

The normal workflow is:

1. Download the `seshat` binary.
2. Run `seshat scan` in each project you want AI to understand.
3. Register those projects in `$HOME/.seshat/config.yml`.
4. Add one MCP entry named `seshat` to Cursor, Codex, Claude, or another MCP client.
5. Tell the AI agent to call `list_projects` and pass `project_id` to Seshat tools.

## Download

Download the latest release from:

[https://github.com/hungnm98/seshat/releases/latest](https://github.com/hungnm98/seshat/releases/latest)

Choose the artifact for your machine:

| Platform | Artifact |
| --- | --- |
| MacBook / macOS universal | `seshat_*_macos_universal.tar.gz` |
| macOS Apple Silicon | `seshat_*_macos_arm64.tar.gz` |
| macOS Intel | `seshat_*_macos_amd64.tar.gz` |
| Linux 64-bit | `seshat_*_linux_amd64.tar.gz` |
| Windows 64-bit | `seshat_*_windows_amd64.zip` |

macOS install:

```bash
mkdir -p $HOME/.local/bin
tar -xzf seshat_*_macos_universal.tar.gz
cp seshat_*/seshat $HOME/.local/bin/seshat
chmod +x $HOME/.local/bin/seshat
seshat version
```

Linux install:

```bash
mkdir -p $HOME/.local/bin
tar -xzf seshat_*_linux_amd64.tar.gz
cp seshat_*/seshat $HOME/.local/bin/seshat
chmod +x $HOME/.local/bin/seshat
seshat version
```

Windows install:

```powershell
Expand-Archive .\seshat_*_windows_amd64.zip -DestinationPath .\seshat-release
mkdir $HOME\.local\bin -Force
copy .\seshat-release\seshat_*\seshat.exe $HOME\.local\bin\seshat.exe
seshat.exe version
```

## Index A Project

Run this once in each repository you want Seshat to understand:

```bash
cd /absolute/path/to/project
seshat init --repo . --config .seshat/project.yaml --project-id my-project
seshat scan --config .seshat/project.yaml --parallel 4
seshat status --config .seshat/project.yaml
```

Use a stable `project_id`, because AI agents will pass that value to Seshat MCP tools.

## Add MCP

Seshat uses one shared MCP server for all local projects. The shared registry is:

```text
$HOME/.seshat/config.yml
```

Register a project:

```bash
seshat mcp add /absolute/path/to/project --registry $HOME/.seshat/config.yml --reload
seshat mcp project ls --registry $HOME/.seshat/config.yml --sort id
```

Generate MCP client config:

```bash
seshat setup --registry $HOME/.seshat/config.yml --client all --print
```

The MCP entry should be named `seshat` and run:

```bash
seshat mcp --registry $HOME/.seshat/config.yml
```

Cursor example:

```json
{
  "mcpServers": {
    "seshat": {
      "command": "/Users/me/.local/bin/seshat",
      "args": ["mcp", "--registry", "/Users/me/.seshat/config.yml"]
    }
  }
}
```

Codex example:

```toml
[mcp_servers.seshat]
command = "/Users/me/.local/bin/seshat"
args = ["mcp", "--registry", "/Users/me/.seshat/config.yml"]
```

Claude example:

```json
{
  "mcpServers": {
    "seshat": {
      "command": "/Users/me/.local/bin/seshat",
      "args": ["mcp", "--registry", "/Users/me/.seshat/config.yml"],
      "type": "stdio"
    }
  }
}
```

## Agent Instructions

Add this to `AGENTS.md`, `CLAUDE.md`, Cursor rules, or the agent instruction file for each workspace:

```md
Use the shared `seshat` MCP server for codebase analysis.

Mandatory:
- Call `list_projects` first when `project_id` is unknown.
- Pass the returned `project_id` to every Seshat code graph tool.
- Run `seshat scan` before non-trivial codebase analysis if the index may be stale, after pulling new code, and after making code changes.
- Use `find_symbol`, `get_symbol_detail`, `find_callers`, `find_callees`, and `file_dependency_graph` before editing or reviewing code.
- Use `file_dependency_graph` before editing a file to understand depends-on and dependent files.
```

## How AI Uses Seshat

Agents should discover projects with:

```text
list_projects()
```

Then use the returned `project_id` with:

```text
find_symbol(project_id, query)
get_symbol_detail(project_id, symbol_id)
find_callers(project_id, symbol_id)
find_callees(project_id, symbol_id)
file_dependency_graph(project_id, file)
```

Before editing a file, agents should call `file_dependency_graph` to understand what the file depends on and what depends on it.

## Refresh The Index

Run this whenever the code changes, after pulling new code, or before serious codebase analysis if the index may be stale:

```bash
cd /absolute/path/to/project
seshat scan --config .seshat/project.yaml --parallel 4
```

Running MCP processes lazily reload updated graph files on their next tool call.

## Local Files

- `.seshat/project.yaml`: project scan config.
- `.seshat/index/graph.json`: local symbol/relation graph.
- `.seshat/index/status.json`: scan status, counts, commit, branch, and language summary.
- `$HOME/.seshat/config.yml`: shared multi-project MCP registry.

Seshat does not upload source code in local V1. The local index stores parsed metadata: files, symbols, relations, and version metadata.

## Development

```bash
go test ./cli/...
go test ./server/...

./scripts/build-local.sh
./scripts/build-release.sh
```

Project layout:

```text
cli/       local scanner, indexer, MCP server, setup helpers, parser packages
server/    later server-backed APIs, storage, workers, and remote MCP gateway
docs/      architecture and CLI/MCP contracts
scripts/   local and release build helpers
```
