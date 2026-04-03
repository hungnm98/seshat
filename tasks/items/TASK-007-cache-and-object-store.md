---
id: TASK-007
title: Redis cache and MinIO object store plumbing
type: feature
status: done
priority: medium
owner: codex
estimate: 3d
depends_on:
  - TASK-001
acceptance_criteria:
  - Redis health checks are wired into runtime services
  - Raw ingestion payloads can be written to object storage
  - Health endpoint reports real dependency status when configured
artifacts:
  - internal/storage/redis
  - internal/storage/objectstore
  - cmd/worker
updated_at: 2026-04-03
---

Add real infrastructure wrappers for cache and object storage services.
