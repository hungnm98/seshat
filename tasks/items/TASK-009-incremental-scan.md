---
id: TASK-009
title: Go-first incremental scan in CLI
type: feature
status: done
priority: high
owner: codex
estimate: 3d
depends_on:
  - TASK-004
acceptance_criteria:
  - CLI can detect changed Go files from git diff in incremental mode
  - Full scan behavior remains unchanged
  - Incremental discovery is covered by tests
artifacts:
  - cmd/cli/main.go
  - internal/parser
updated_at: 2026-04-03
---

Implement changed-file discovery and targeted analysis for Go-first incremental runs.
