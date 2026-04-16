---
id: TASK-015
title: Implement local MCP stdio server in CLI
type: feature
status: done
priority: high
owner: codex
estimate: 2d
depends_on:
  - TASK-014
acceptance_criteria:
  - seshat mcp serves initialize, tools/list, and tools/call over stdio
  - MCP tools read from local graph.json and require project_id
  - Missing index, invalid project_id, unknown methods, and missing symbols return clear errors
artifacts:
  - cli/internal/mcp
  - cli/cmd/seshat/main.go
updated_at: 2026-04-17
---

Expose the local Seshat graph through MCP stdio for Codex, Cursor, and compatible MCP clients.
