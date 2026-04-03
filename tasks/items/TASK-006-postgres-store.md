---
id: TASK-006
title: PostgreSQL-backed storage adapter
type: feature
status: done
priority: high
owner: codex
estimate: 4d
depends_on:
  - TASK-001
  - TASK-002
  - TASK-003
acceptance_criteria:
  - Store can persist projects, tokens, ingestion runs, versions, symbols, and relations in PostgreSQL
  - App can select postgres store via config
  - Query and auth paths work against persistent storage
artifacts:
  - internal/storage/postgres
  - internal/app/app.go
updated_at: 2026-04-03
---

Replace the in-memory-only runtime path with a compile-ready PostgreSQL adapter.
