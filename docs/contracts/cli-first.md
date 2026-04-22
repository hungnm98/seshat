# CLI-First Local Index Contract

Seshat V1 can run without the server. The CLI parses a repository, writes a local JSON graph, and exposes that graph through a local MCP stdio server for tools such as Codex, Cursor, and Claude.

## Commands

```bash
seshat init
seshat ingest [--all] [--parallel 1] [-v] [--dry-run] [--json]
seshat inspect [--json] [--reparse]
seshat status [--json]
seshat mcp --registry $HOME/.seshat/config.yml
seshat mcp add {folder_project} [--registry $HOME/.seshat/config.yml] [--reload]
seshat mcp reload [--registry $HOME/.seshat/config.yml]
seshat mcp project ls [--registry $HOME/.seshat/config.yml] [--sort id|path] [--json]
seshat mcp config path|show|edit [--registry $HOME/.seshat/config.yml]
seshat graph --file path/to/file.go [--format mermaid|dot|json]
seshat setup --registry $HOME/.seshat/config.yml [--client cursor|codex|claude|all] [--print]
```

`login` is intentionally out of scope for local-only V1. Existing server-backed `auth verify` and `dependencies` commands remain for compatibility, but the local workflow does not require them.

`ingest` parses files with one worker by default. Increase `--parallel` or use the `--threads` alias for large repositories. It prints basic progress logs by default; add `-v` or `--verbose` for target-level timing and error context.

## Local Files

- `.seshat/project.yaml`: canonical project config.
- `.seshat/index/graph.json`: latest local `AnalysisBatch`.
- `.seshat/index/status.json`: counts, language summary, commit, branch, scan mode, generated time, and config hash.

`graph.json` stores symbols and relations only. It does not store source code.

## MCP Tools

`seshat mcp --registry $HOME/.seshat/config.yml` serves all registered projects from one MCP entry named `seshat`. This is the default agent integration for Codex, Cursor, Claude, and other MCP clients.

Discovery tools:

- `seshat_info()`
- `list_projects()`

Code graph tools:

- `find_symbol(project_id, query, kind?, limit?)`
- `get_symbol_detail(project_id, symbol_id)`
- `find_callers(project_id, symbol_id, depth?)`
- `find_callees(project_id, symbol_id, depth?)`
- `file_dependency_graph(project_id, file, depth?, direction?, max_files?, compact?)`

Every code graph tool requires `project_id`. In registry mode, agents should call `list_projects` first and use one of the returned project ids. Query depth defaults to `1` and is capped at `3`; result limit defaults to `50`.

`file_dependency_graph` supports `direction` values `both`, `depends-on`, and `dependents`. It defaults to compact output, capped at 25 files per direction, so MCP clients get a usable map instead of a full graph dump. Pass `compact:false` only when nested symbols and relations are needed.

Go call edges include direct calls plus a conservative heuristic for nested selector calls such as `c.services.OrderService.CreateOrder()`. Heuristic edges are marked with relation metadata `resolution: heuristic_selector_method` and should be verified with `rg` for high-impact refactors.

## Local Multi-Project MCP

`seshat mcp --registry $HOME/.seshat/config.yml` is the default workflow for machines that work with multiple indexed repositories. It still uses stdio transport; there is no HTTP daemon in local V1.

The default registry is `$HOME/.seshat/config.yml`:

```yaml
projects:
  - id: seshat
    path: /path/to/repo
    config: /path/to/repo/.seshat/project.yaml
cache:
  idle_ttl: 30m
```

Commands:

- `seshat mcp add {folder_project}` registers `{folder_project}/.seshat/project.yaml`.
- `seshat mcp add {folder_project} --reload` registers the project and validates the registry; each stdio MCP process lazily reloads registry and graph files on its next tool call.
- `seshat mcp reload` validates the registry. It does not broadcast to already-running stdio processes.
- `seshat mcp project ls --sort id|path` prints registered projects in a stable order.
- `seshat mcp config path|show|edit` helps inspect and edit the registry.

`cache.idle_ttl` controls how long an unused project graph query index stays in RAM. The default is `30m`; set it to `0s` to keep loaded project graphs until the MCP process exits. The cache is an in-process map keyed by `project_id`, and each graph load validates that the graph metadata matches the requested project before answering.

Because stdio MCP is process-local, each agent usually owns its own `seshat mcp` process and cache. Registry and graph reloads are lazy: the process checks file `mtime`/size on tool calls and reloads changed files before answering.

## Dependency Charts

`seshat graph` renders the local file dependency graph without calling the server:

```bash
seshat graph --file cli/cmd/seshat/main.go --format mermaid
seshat graph --file cli/cmd/seshat/main.go --format dot --direction dependents
seshat graph --file cli/cmd/seshat/main.go --format json
```

Supported formats:

- `mermaid`: paste into Markdown.
- `dot`: render with Graphviz.
- `json`: inspect the normalized chart view.

Supported directions are `both`, `depends-on`, and `dependents`. Internal Go imports are mapped to package files when the import path belongs to the local CLI or server module. Test files are excluded from import dependency edges by default, but can still appear as dependents through call relations.

## Setup

`seshat setup --registry $HOME/.seshat/config.yml --client all --print` prints one MCP entry named `seshat` for all registered projects:

```bash
seshat mcp --registry /absolute/path/to/.seshat/config.yml
```

Generated instructions tell agents to use the `seshat` MCP server, call `list_projects` to discover `project_id`, and pass `project_id` to every code query.

The setup command prints snippets by default and does not write secrets. Local V1 does not require tokens.
