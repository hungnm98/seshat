---
id: TASK-016
title: Implement setup for Cursor, Codex, and Claude
type: feature
status: done
priority: medium
owner: codex
estimate: 1d
depends_on:
  - TASK-015
acceptance_criteria:
  - seshat setup prints Cursor MCP config
  - seshat setup prints Codex MCP config
  - seshat setup prints Claude MCP config
  - Generated config points to seshat mcp with an absolute project config path
artifacts:
  - cli/internal/setup
  - cli/cmd/seshat/main.go
updated_at: 2026-04-17
---

Generate local MCP configuration snippets for Cursor, Codex, and Claude without writing secrets.
