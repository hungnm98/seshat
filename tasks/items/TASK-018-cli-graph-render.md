---
id: TASK-018
title: Render local dependency graph charts from CLI
type: feature
status: done
priority: medium
owner: codex
estimate: 1d
depends_on:
  - TASK-014
acceptance_criteria:
  - seshat graph renders Mermaid output for a project-relative file
  - seshat graph renders DOT output for Graphviz
  - Chart output supports depends-on, dependents, and both directions
  - Go package symbol ids avoid cross-package collisions for duplicate package names
artifacts:
  - cli/internal/graphrender
  - cli/internal/parser/golang/analyzer.go
  - cli/cmd/seshat/main.go
updated_at: 2026-04-17
---

Add direct graph chart rendering from the local JSON index and tighten Go symbol namespaces so file dependency charts avoid duplicate package-name collisions.
