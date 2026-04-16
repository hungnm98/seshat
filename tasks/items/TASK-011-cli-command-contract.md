---
id: TASK-011
title: Redesign Seshat CLI command contract
type: feature
status: done
priority: high
owner: codex
estimate: 1d
depends_on:
  - TASK-010
acceptance_criteria:
  - Local-only command behavior is documented for init, ingest, push, watch, inspect, status, mcp, and setup
  - Server upload, login, and token auth are explicitly out of scope for local V1
artifacts:
  - docs/contracts/cli-first.md
updated_at: 2026-04-17
---

Define the CLI-first command contract inspired by Grapuco while preserving Seshat project_id and graph schema compatibility.
