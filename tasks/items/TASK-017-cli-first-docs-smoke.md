---
id: TASK-017
title: CLI-first docs and smoke demo
type: chore
status: done
priority: medium
owner: codex
estimate: 1d
depends_on:
  - TASK-015
  - TASK-016
acceptance_criteria:
  - README documents local first-run, daily push/watch, inspect/status, MCP, and setup flows
  - Automated smoke tests cover init, ingest, inspect, status, and MCP tool calls
artifacts:
  - README.md
  - cli/cmd/seshat/main_test.go
updated_at: 2026-04-17
---

Document and validate the local-only CLI workflow from a clean repository to MCP tool calls.
