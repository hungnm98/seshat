# Workspace Instructions

This repository defines the Seshat workflow for Vikunja-managed tasks.

## When These Instructions Apply

- Apply these instructions when the request involves Vikunja tasks, projects, labels, buckets, comments, reviews, or workflow stages.
- Treat `.cursor/rules/` as Cursor-only configuration. The Codex-compatible instructions for this repo live in this file and in `.agents/skills/`.

## Primary Workflow

- Use the `vikunja-flow` skill at `.agents/skills/vikunja-flow/SKILL.md` for any Vikunja workflow request.
- Keep the flow aligned with `docs/vikunja-agent-workflow.md`.
- Reuse the prompt library in `prompts/` when acting as PM/Senior AI, Senior Dev, Reviewer, or when preparing a hard-task handoff.
- Use `seshat_project_proposal.md` and `docs/seshat-project-plan.md` as the project baseline.

## Working Rules

- Do not skip workflow stages, task comments, or required artifacts when a task changes hands.
- If the task is unclear or blocked, prefer clarification or blocked state instead of guessing.
- If repository work is needed, clone into `repos/` and use one folder per task with the pattern `{gh_project_name}-{vikunja_project_name}-{task_id}`.
- Use a dedicated git branch for each task.
- For difficult tasks, Codex can assist with implementation, but Senior Dev remains responsible for validating the result before review.
- Preserve the Seshat architecture baseline unless the user explicitly changes it: local/CI CLI analysis, Go server, PostgreSQL/Redis/object storage, shared parser schema, and one multi-project MCP gateway with `project_id`.

## Files To Consult

- Workflow reference: `docs/vikunja-agent-workflow.md`
- PM / Senior AI prompt: `prompts/ba-techlead.md`
- Senior Dev prompt: `prompts/senior-dev.md`
- Reviewer prompt: `prompts/reviewer.md`
- Hard-task handoff prompt: `prompts/hard-task-handoff.md`
- Project plan: `docs/seshat-project-plan.md`
