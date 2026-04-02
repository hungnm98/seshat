---
id: TASK-004
title: Parser foundation for Go and Ruby
type: feature
status: done
priority: high
owner: codex
estimate: 4d
depends_on:
  - TASK-001
acceptance_criteria:
  - Go parser extracts symbols and call relations from fixture
  - Ruby parser extracts module/class/method symbols from fixture
artifacts:
  - internal/parser/golang/analyzer.go
  - internal/parser/ruby/analyzer.go
updated_at: 2026-04-02
---

Language adapters that normalize source analysis into graphschema v1.
