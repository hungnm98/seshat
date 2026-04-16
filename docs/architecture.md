# Seshat Architecture

## Runtime Overview

- `cli/cmd/seshat` scans a local repository, builds an `AnalysisBatch`, uploads it to the server, and exposes agent-facing CLI queries.
- `server/cmd/server` serves REST APIs, the admin UI, token verification, ingestion, and graph queries.
- `server/cmd/worker` is reserved for batch merge, cache warming, and future background processing.
- `server/cmd/mcp` publishes the initial MCP tool contract for project-scoped graph queries.

## Core Flows

### Admin

1. Bootstrap admin logs in at `/admin/login`.
2. Admin creates a project.
3. Admin generates a project-scoped token.
4. Admin monitors ingestion runs, audit logs, and system health.

### CLI Ingestion

1. CLI loads `.seshat/project.yaml`.
2. CLI scans Go and Ruby files based on include/exclude paths.
3. CLI uploads a merged `AnalysisBatch` to `POST /api/v1/projects/{project_id}/ingestions`.
4. Server validates `graphschema v1` and stores a project version plus ingestion run.

### Query

1. A client calls a project-scoped query endpoint with the project token.
2. The query service resolves the latest project version.
3. The server returns matching symbols or depth-bounded callers/callees.

### File Dependency Graph

1. An agent or CLI calls `GET /api/v1/projects/{project_id}/graph/dependencies?file={path}&depth={n}`.
2. The query service resolves the latest project version.
3. The graph builder finds symbols declared in the requested file.
4. It traverses symbol relations in both directions:
   - `depends_on`: files the requested file calls, references, imports, or implements.
   - `dependents`: files that call, reference, import, or implement symbols from the requested file.
5. The response includes root file symbols, related files, symbols, relations, relation reasons, depth, and version metadata.

CLI usage:

```bash
cd cli
go run ./cmd/seshat dependencies --config ../.seshat/project.yaml --file internal/order/service.go --depth 1
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
