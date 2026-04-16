# Seshat

Seshat is a Go-based code knowledge graph and AI context engine. The current V1 workflow is CLI-first: parse a repository locally, store a JSON graph index, and expose that index through a local MCP stdio server for Codex, Cursor, and other MCP clients.

- `cli/` for local scanning, parsing, JSON indexing, MCP, and agent-facing queries
- `server/` for later ingestion, query APIs, admin UI, token auth, worker jobs, and MCP gateway work
- Go + Ruby parser adapters
- Git-native task management in `tasks/`

## Quick Start

Run the local-only flow from the repository you want to index:

```bash
cd cli
go run ./cmd/seshat init --repo .. --config ../.seshat/project.yaml --project-id seshat
go run ./cmd/seshat ingest --config ../.seshat/project.yaml
go run ./cmd/seshat status --config ../.seshat/project.yaml
go run ./cmd/seshat inspect --config ../.seshat/project.yaml --json
go run ./cmd/seshat graph --config ../.seshat/project.yaml --file cli/cmd/seshat/main.go --format mermaid
```

Start a local MCP stdio server over the saved JSON index:

```bash
cd cli
go run ./cmd/seshat mcp --config ../.seshat/project.yaml
```

Generate MCP client config snippets:

```bash
cd cli
go run ./cmd/seshat setup --config ../.seshat/project.yaml --client all --print
```

## Daily CLI Flow

```bash
go run ./cmd/seshat push --config ../.seshat/project.yaml
go run ./cmd/seshat watch --config ../.seshat/project.yaml --debounce 2000
```

`push` updates the local JSON index from changed files. `watch` polls supported source files and runs local push after changes.

## Local Files

- `.seshat/project.yaml` stores local project config.
- `.seshat/index/graph.json` stores the latest `AnalysisBatch`.
- `.seshat/index/status.json` stores counts, commit metadata, language summary, scan mode, and config hash.

## MVP Notes

- Local V1 does not require login, API tokens, PostgreSQL, Redis, MinIO, or a running server.
- The CLI does not upload source code. The local index stores parsed metadata: files, symbols, relations, and version metadata.
- Existing server code remains in the repo for future remote ingestion and gateway work.
- See `docs/contracts/cli-first.md` for the command and MCP contract.

## Commands

```bash
go test ./cli/...
go test ./server/...

cd cli
go run ./cmd/seshat init --repo .. --config ../.seshat/project.yaml
go run ./cmd/seshat ingest --config ../.seshat/project.yaml
go run ./cmd/seshat push --config ../.seshat/project.yaml
go run ./cmd/seshat inspect --config ../.seshat/project.yaml --json
go run ./cmd/seshat status --config ../.seshat/project.yaml --json
go run ./cmd/seshat graph --config ../.seshat/project.yaml --file cli/cmd/seshat/main.go --format mermaid
go run ./cmd/seshat setup --config ../.seshat/project.yaml --client cursor --print
```

## Project Layout

```text
cli/                 CLI project and Go module
cli/cmd/seshat/      CLI binary
cli/internal/        Local index, query, MCP, setup, watch, parser packages
cli/pkg/             CLI graph schema and model contracts
server/              Backend project and Go module
server/cmd/server/   HTTP server binary
server/cmd/worker/   Background worker binary
server/cmd/mcp/      MCP metadata binary
server/internal/     Admin, API, auth, ingestion, query, and storage
server/pkg/          Server graph schema and model contracts
tasks/               Git-native backlog, roadmap, and execution workflow
```
