# Seshat

Seshat is a Go-based code knowledge graph and AI context engine skeleton. The workspace is split into two independent Go projects:

- `cli/` for local scanning, parsing, upload, and agent-facing queries
- `server/` for ingestion, query APIs, admin UI, token auth, worker jobs, and MCP metadata
- Go + Ruby parser adapters
- Git-native task management in `tasks/`

## Quick Start

1. Start the optional local services:

```bash
docker compose -f server/deployments/docker/docker-compose.yml up -d
```

2. Run the server:

```bash
cd server
go run ./cmd/server
```

To run against PostgreSQL instead of the in-memory MVP store:

```bash
export SESHAT_STORE_KIND=postgres
export SESHAT_POSTGRES_DSN='postgres://seshat:seshat@localhost:5432/seshat?sslmode=disable'
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
cd cli
go run ./cmd/seshat scan --config ../.seshat/project.yaml --mode full
```

## MVP Notes

- Runtime storage defaults to in-memory for the bootstrap skeleton.
- Setting `SESHAT_STORE_KIND=postgres` switches the server to the Postgres adapter and auto-applies the MVP schema at startup.
- `go-admin` integration can be enabled with `SESHAT_GOADMIN_ENABLED=true` and `SESHAT_POSTGRES_DSN` configured. The custom admin UI under `/admin/` remains the primary management surface for the MVP.
- PostgreSQL, Redis, and MinIO are scaffolded for local development and future storage adapters.

## Commands

```bash
go test ./cli/...
go test ./server/...

cd cli
go run ./cmd/seshat auth verify --config ../.seshat/project.yaml
go run ./cmd/seshat dependencies --config ../.seshat/project.yaml --file internal/order/service.go --depth 1

cd ../server
go run ./cmd/mcp
```

## Project Layout

```text
cli/                 CLI project and Go module
cli/cmd/seshat/      CLI binary
cli/internal/parser/ Go and Ruby analyzers
cli/pkg/             CLI graph schema and model contracts
server/              Backend project and Go module
server/cmd/server/   HTTP server binary
server/cmd/worker/   Background worker binary
server/cmd/mcp/      MCP metadata binary
server/internal/     Admin, API, auth, ingestion, query, and storage
server/pkg/          Server graph schema and model contracts
tasks/               Git-native backlog, roadmap, and execution workflow
```
