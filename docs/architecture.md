# Seshat Architecture

## Runtime Overview

- `cmd/cli` scans a local repository, builds an `AnalysisBatch`, and uploads it to the server using a project-scoped bearer token.
- `cmd/server` serves REST APIs, the admin UI, token verification, ingestion, and graph queries.
- `cmd/worker` is reserved for batch merge, cache warming, and future background processing.
- `cmd/mcp` publishes the initial MCP tool contract for project-scoped graph queries.

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
