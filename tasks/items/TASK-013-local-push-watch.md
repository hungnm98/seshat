---
id: TASK-013
title: Implement local push and watch
type: feature
status: done
priority: high
owner: codex
estimate: 2d
depends_on:
  - TASK-012
acceptance_criteria:
  - seshat push updates the local index from changed files
  - seshat push --force performs a full re-index
  - seshat watch ignores generated and dependency directories
artifacts:
  - cli/cmd/seshat/main.go
  - cli/internal/watch
updated_at: 2026-04-17
---

Implement local incremental indexing and a polling watch loop for day-to-day development.
