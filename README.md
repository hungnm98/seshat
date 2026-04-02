# Seshat

Seshat is a Go-based code knowledge graph and AI context engine skeleton. This bootstrap focuses on a compile-ready MVP with:

- `cli` for local scanning and upload
- `server` for ingestion, query APIs, admin UI, and token auth
- `worker` skeleton for background jobs
- `mcp` skeleton exposing v1 tool metadata
- Go + Ruby parser adapters
- Git-native task management in `tasks/`

## Quick Start

1. Start the optional local services:

```bash
docker compose -f deployments/docker/docker-compose.yml up -d
```

2. Run the server:

```bash
go run ./cmd/server
```

3. Open the admin UI:

- URL: [http://localhost:8080/admin/login](http://localhost:8080/admin/login)
- Default username: `admin`
- Default password: `admin123`

4. Create a project and token in the admin UI.

5. Configure the CLI:

```bash
cp .seshat/project.yaml.example .seshat/project.yaml
export SESHAT_PROJECT_TOKEN="paste-token-from-admin"
go run ./cmd/cli scan --config .seshat/project.yaml --mode full
```

## MVP Notes

- Runtime storage defaults to in-memory for the bootstrap skeleton.
- `go-admin` integration can be enabled with `SESHAT_GOADMIN_ENABLED=true` and `SESHAT_POSTGRES_DSN` configured. The custom admin UI under `/admin/` remains the primary management surface for the MVP.
- PostgreSQL, Redis, and MinIO are scaffolded for local development and future storage adapters.

## Commands

```bash
go build ./...
go test ./...
go run ./cmd/cli auth verify --config .seshat/project.yaml
go run ./cmd/mcp
```

## Project Layout

```text
cmd/                 Runtime binaries
internal/admin/      Admin pages, session auth, optional go-admin hook
internal/api/        HTTP routes and middleware
internal/parser/     Go and Ruby analyzers
internal/storage/    Store interface + memory MVP adapter
pkg/model/           Shared graph, token, admin, and project models
tasks/               Git-native backlog, roadmap, and execution workflow
```
