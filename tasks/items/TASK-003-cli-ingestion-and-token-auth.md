---
id: TASK-003
title: CLI ingestion and project token auth
type: feature
status: done
priority: high
owner: codex
estimate: 3d
depends_on:
  - TASK-001
  - TASK-002
acceptance_criteria:
  - CLI verifies project token
  - CLI can upload an analysis batch
  - Server rejects tokens for the wrong project
artifacts:
  - cmd/cli/main.go
  - internal/auth/service.go
updated_at: 2026-04-02
---

Project-scoped bearer token flow for ingestion and query APIs.
