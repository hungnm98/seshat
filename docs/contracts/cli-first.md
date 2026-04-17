# CLI-First Local Index Contract

Seshat V1 can run without the server. The CLI parses a repository, writes a local JSON graph, and exposes that graph through a local MCP stdio server for tools such as Codex, Cursor, and Claude.

## Commands

```bash
seshat init
seshat ingest [--all] [--parallel 1] [-v] [--dry-run] [--json]
seshat push [--force]
seshat watch [--debounce 2000]
seshat inspect [--json] [--reparse]
seshat status [--json]
seshat mcp
seshat graph --file path/to/file.go [--format mermaid|dot|json]
seshat setup [--client cursor|codex|claude|all] [--print]
```

`login` is intentionally out of scope for local-only V1. Existing server-backed `auth verify` and `dependencies` commands remain for compatibility, but the local workflow does not require them.

`ingest` parses files with one worker by default. Increase `--parallel` or use the `--threads` alias for large repositories. It prints basic progress logs by default; add `-v` or `--verbose` for target-level timing and error context.

## Local Files

- `.seshat/project.yaml`: canonical project config.
- `.seshat/index/graph.json`: latest local `AnalysisBatch`.
- `.seshat/index/status.json`: counts, language summary, commit, branch, scan mode, generated time, and config hash.

`graph.json` stores symbols and relations only. It does not store source code.

## MCP Tools

`seshat mcp --config .seshat/project.yaml` serves JSON-RPC over stdio and exposes:

- `find_symbol(project_id, query, kind?, limit?)`
- `get_symbol_detail(project_id, symbol_id)`
- `find_callers(project_id, symbol_id, depth?)`
- `find_callees(project_id, symbol_id, depth?)`
- `file_dependency_graph(project_id, file, depth?, direction?, max_files?, compact?)`

Every tool requires `project_id`. In local V1, the value must match `.seshat/project.yaml`. Query depth defaults to `1` and is capped at `3`; result limit defaults to `50`.

`file_dependency_graph` supports `direction` values `both`, `depends-on`, and `dependents`. It defaults to compact output, capped at 25 files per direction, so MCP clients get a usable map instead of a full graph dump. Pass `compact:false` only when nested symbols and relations are needed.

Go call edges include direct calls plus a conservative heuristic for nested selector calls such as `c.services.OrderService.CreateOrder()`. Heuristic edges are marked with relation metadata `resolution: heuristic_selector_method` and should be verified with `rg` for high-impact refactors.

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

`seshat setup --client all --print` prints Cursor, Codex, and Claude MCP snippets that run:

```bash
seshat mcp --config /absolute/path/to/.seshat/project.yaml
```

The setup command prints snippets by default and does not write secrets. Local V1 does not require tokens.
