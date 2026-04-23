# Seshat

Seshat is a Go-based code knowledge graph and AI context engine. The current V1 workflow is CLI-first: parse repositories locally, store JSON graph indexes, and expose them through one shared local MCP stdio server named `seshat` for Codex, Cursor, Claude, and other MCP clients.

- `cli/` for local scanning, parsing, JSON indexing, MCP, and agent-facing queries
- `server/` for later ingestion, query APIs, admin UI, token auth, worker jobs, and MCP gateway work
- Go + Ruby parser adapters
- Git-native task management in `tasks/`

## Quick Start

Run the local-only flow from the repository you want to index:

```bash
cd cli
go run ./cmd/seshat init --repo .. --config ../.seshat/project.yaml --project-id seshat
go run ./cmd/seshat ingest --config ../.seshat/project.yaml --parallel 1
go run ./cmd/seshat status --config ../.seshat/project.yaml
go run ./cmd/seshat inspect --config ../.seshat/project.yaml --json
go run ./cmd/seshat graph --config ../.seshat/project.yaml --file cli/cmd/seshat/main.go --format mermaid
```

Register the project in the shared local MCP registry:

```bash
cd cli
go run ./cmd/seshat mcp add .. --registry $HOME/.seshat/config.yml
go run ./cmd/seshat mcp project ls --registry $HOME/.seshat/config.yml --sort id
```

Generate one MCP client config entry named `seshat`:

```bash
cd cli
go run ./cmd/seshat setup --registry $HOME/.seshat/config.yml --client all --print
```

Start the shared MCP stdio server:

```bash
cd cli
go run ./cmd/seshat mcp --registry $HOME/.seshat/config.yml
```

Agents discover projects by calling `list_projects` on the `seshat` MCP server, then pass the returned `project_id` to code graph tools. The registry supports `cache.idle_ttl` (default `30m`) so unused project indexes are released from RAM while the MCP process keeps running. Each stdio MCP process checks registry and graph file changes on tool calls, so reload is lazy per process rather than broadcast.

## Refreshing The Index

```bash
go run ./cmd/seshat scan --config ../.seshat/project.yaml --parallel 4
```

`scan` refreshes the local JSON index from the repository. Running MCP processes lazily reload the updated graph on their next tool call.

## Local Files

- `.seshat/project.yaml` stores local project config.
- `$HOME/.seshat/config.yml` stores the shared multi-project MCP registry.
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

./scripts/build-local.sh
./scripts/build-local.sh /tmp/seshat
./scripts/build-release.sh

cd cli
go run ./cmd/seshat init --repo .. --config ../.seshat/project.yaml
go run ./cmd/seshat ingest --config ../.seshat/project.yaml --parallel 4 -v
go run ./cmd/seshat inspect --config ../.seshat/project.yaml --json
go run ./cmd/seshat status --config ../.seshat/project.yaml --json
go run ./cmd/seshat mcp -h
go run ./cmd/seshat graph --config ../.seshat/project.yaml --file cli/cmd/seshat/main.go --format mermaid
go run ./cmd/seshat setup --registry $HOME/.seshat/config.yml --client claude --print
```

## Project Layout

```text
cli/                 CLI project and Go module
cli/cmd/seshat/      CLI binary
cli/internal/        Local index, query, MCP, setup, parser packages
cli/pkg/             CLI graph schema and model contracts
server/              Backend project and Go module
server/cmd/server/   HTTP server binary
server/cmd/worker/   Background worker binary
server/cmd/mcp/      MCP metadata binary
server/internal/     Admin, API, auth, ingestion, query, and storage
server/pkg/          Server graph schema and model contracts
tasks/               Git-native backlog, roadmap, and execution workflow
```
