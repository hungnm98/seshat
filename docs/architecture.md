# Seshat Architecture

## Runtime Overview

- `cli/cmd/seshat` scans local repositories, builds `AnalysisBatch` indexes, writes `.seshat/index`, and serves one shared multi-project MCP stdio entry through `seshat mcp --registry $HOME/.seshat/config.yml`.
- `server/cmd/server` serves REST APIs, the admin UI, token verification, ingestion, and graph queries for the later server-backed workflow.
- `server/cmd/worker` is reserved for batch merge, cache warming, and future background processing.
- `server/cmd/mcp` publishes the server-side MCP metadata contract for future remote gateway work.

## Core Flows

### Admin

1. Bootstrap admin logs in at `/admin/login`.
2. Admin creates a project.
3. Admin generates a project-scoped token.
4. Admin monitors ingestion runs, audit logs, and system health.

### CLI Ingestion

1. CLI loads `.seshat/project.yaml`.
2. CLI scans Go and Ruby files based on include/exclude paths.
3. CLI writes `.seshat/index/graph.json` and `.seshat/index/status.json`.
4. `seshat mcp add {repo} --registry $HOME/.seshat/config.yml` registers the project in the shared local MCP registry.

The older server upload path remains part of the long-term architecture, but local V1 does not require login, tokens, or a running server.

### Query

1. An MCP client starts one `seshat` entry with `seshat mcp --registry $HOME/.seshat/config.yml`.
2. The agent calls `list_projects` to discover available `project_id` values.
3. The agent passes `project_id` to `find_symbol`, `get_symbol_detail`, `find_callers`, `find_callees`, or `file_dependency_graph`.
4. The registry-backed MCP server loads the matching local project graph, validates that graph metadata matches the requested `project_id`, and returns bounded structured results.

### File Dependency Graph

1. An agent calls the `file_dependency_graph` MCP tool with `project_id`, `file`, and optional `depth`.
2. The local query provider resolves the latest graph for that project.
3. The graph builder finds symbols declared in the requested file.
4. It traverses symbol relations in both directions:
   - `depends_on`: files the requested file calls, references, imports, or implements.
   - `dependents`: files that call, reference, import, or implement symbols from the requested file.
5. The response includes root file symbols, related files, symbols, relations, relation reasons, depth, and version metadata.

CLI usage:

```bash
cd cli
go run ./cmd/seshat mcp --registry $HOME/.seshat/config.yml
go run ./cmd/seshat graph --config ../.seshat/project.yaml --file cli/cmd/seshat/main.go --format mermaid
```

This is the primary query shape for agents before editing a file because it answers:

- what this file depends on
- what depends on this file
- which symbols and relation types connect those files
- which tests or callers are likely worth inspecting next

## Storage Strategy

- MVP runtime uses the in-memory store for buildability and fast tests.
- The runtime can now switch to a Postgres-backed adapter through `SESHAT_STORE_KIND=postgres` and `SESHAT_POSTGRES_DSN`.
- Compose and migrations prepare PostgreSQL, Redis, and MinIO for the next implementation phase.
- Raw analysis payloads are retained in the store abstraction so object storage can be introduced without changing API contracts.

## Admin Strategy

- Custom server-rendered admin pages under `/admin/` provide the operational workflow for MVP.
- Optional `go-admin` integration is mounted under `/admin/ui` when PostgreSQL is configured.
- Session auth is cookie-based for the admin web surface.
- CLI auth is bearer-token based and project-scoped.
