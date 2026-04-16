---
id: TASK-014
title: Implement local query engine over JSON graph
type: feature
status: done
priority: high
owner: codex
estimate: 2d
depends_on:
  - TASK-012
acceptance_criteria:
  - Local query supports symbol search, symbol detail, callers, callees, and file dependency graph
  - Query depth and result limits are bounded
  - Wrong project_id is rejected
artifacts:
  - cli/internal/localquery
updated_at: 2026-04-17
---

Add deterministic local queries over .seshat/index/graph.json without calling the server.
