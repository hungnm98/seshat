---
id: TASK-010
title: API and admin integration coverage
type: chore
status: done
priority: medium
owner: codex
estimate: 2d
depends_on:
  - TASK-002
  - TASK-003
acceptance_criteria:
  - Admin login, project create, token issue, auth verify, ingestion, and query flows are covered by HTTP tests
  - Regression bugs in owned admin/API files are fixed
artifacts:
  - internal/api
  - internal/admin
updated_at: 2026-04-03
---

Expand automated integration coverage around the current operational workflow.
