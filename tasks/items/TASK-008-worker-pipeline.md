---
id: TASK-008
title: Worker pipeline for ingestion follow-up jobs
type: feature
status: done
priority: medium
owner: codex
estimate: 3d
depends_on:
  - TASK-006
  - TASK-007
acceptance_criteria:
  - Worker process performs a meaningful ingestion follow-up loop or one-shot run
  - Cache warming or payload persistence hooks are executed from worker code
  - Worker logs and health behavior are documented
artifacts:
  - cmd/worker
  - docs/runbooks
updated_at: 2026-04-03
---

Move post-ingestion responsibilities into the worker lane.
