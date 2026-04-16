---
id: TASK-012
title: Implement local init, ingest, inspect, and status
type: feature
status: done
priority: high
owner: codex
estimate: 2d
depends_on:
  - TASK-011
acceptance_criteria:
  - Fresh repo can run seshat init without a server
  - seshat ingest writes .seshat/index/graph.json and .seshat/index/status.json
  - seshat inspect and seshat status report local metadata and counts
artifacts:
  - cli/cmd/seshat/main.go
  - cli/internal/localindex
updated_at: 2026-04-17
---

Add local project initialization, full local indexing, index audit, and status reporting.
