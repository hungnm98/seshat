---
id: TASK-002
title: Admin auth and dashboard
type: feature
status: done
priority: high
owner: codex
estimate: 3d
depends_on:
  - TASK-001
acceptance_criteria:
  - Admin can log in with bootstrap credentials
  - Admin can create projects and tokens
  - Dashboard shows health and ingestion status
artifacts:
  - internal/admin/service.go
  - internal/api/router.go
updated_at: 2026-04-02
---

Server-rendered admin pages with cookie session auth and token operations.
