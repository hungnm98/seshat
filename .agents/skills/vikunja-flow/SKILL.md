---
name: "vikunja-flow"
description: "Use for Seshat Vikunja workflow: PM/Senior AI discovery, project planning, architecture handoff, implementation, review, task comments, and stage transitions."
---

# Vikunja Flow For Seshat

Use this skill when the request involves Vikunja tasks, project planning, role handoffs, comments, buckets, labels, or execution workflow for the Seshat project.

Seshat is a Code Knowledge Graph + AI Context Engine. The current source of truth for product and architecture direction is `seshat_project_proposal.md`.

## Goal

Keep Codex aligned with Seshat's workflow so discovery, planning, implementation, review, and owner handoff produce useful artifacts instead of isolated comments.

Codex is the agent that performs the requested workflow step in this repo. When the user names a role such as PM, Senior AI, Senior Dev, or Reviewer, Codex should produce that role's required artifact directly.

## Read Only What You Need

1. Start with `docs/vikunja-agent-workflow.md`.
2. For project discovery or planning, read `seshat_project_proposal.md`.
3. Open only the prompt file that matches the current role:
   - `prompts/ba-techlead.md` for PM + Senior AI discovery/planning.
   - `prompts/senior-dev.md` for implementation planning and coding.
   - `prompts/reviewer.md` for review.
   - `prompts/hard-task-handoff.md` for split work or handoff to another executor.
4. Use workflow defaults only if a local config file is added later and bucket/label names need confirmation.

## Seshat Product Baseline

Keep these decisions stable unless the user explicitly changes them:

- Product: Code Knowledge Graph + AI Context Engine.
- Local CLI scans repositories and uploads normalized analysis payloads.
- Server owns ingestion, validation, graph merge, query engine, context builder, and MCP gateway.
- MVP uses one multi-project MCP gateway. Do not plan a separate MCP process per project unless requested.
- Every MCP tool call must include `project_id`.
- Recommended stack: Go server, PostgreSQL, Redis, object storage, language parser adapters.
- Language priority: Ruby and Go first, then TypeScript/JavaScript, then Java.
- MVP 1 tools: `find_symbol`, `get_symbol_detail`, `find_callers`, `find_callees`.
- CLI sends raw normalized symbols/relations; server builds derived graph and indexes.

## Core Behavior

- Prefer Vikunja MCP tools before suggesting manual task or board updates when those tools are available.
- If the request maps to a workflow role, produce the role artifact in a form that can be pasted into a Vikunja comment.
- For planning requests, start from `seshat_project_proposal.md` and turn it into milestones, risks, dependencies, acceptance criteria, and implementation prompts.
- If a task is mentioned without enough context, gather task ID, project, labels, bucket, description, comments, and recent outputs.
- Treat labels as workflow signals and buckets as visible stage state.
- Preserve required stage order and required outputs.
- If information is missing, call it out clearly instead of inventing facts.
- If the task is blocked or ambiguous, recommend clarification or `ai:blocked`.

## Stage Rules

- PM / Senior AI planning output must exist before `ai:dev-ready`.
- Senior Dev output must exist before `ai:review-ready`.
- Reviewer output must exist before `ai:owner-ready`.
- Do not recommend moving a task forward if the required handoff package is incomplete.

## Role Mapping

### PM / Senior AI Discovery

Use `prompts/ba-techlead.md`.

Codex should produce:
- Project understanding from `seshat_project_proposal.md`
- Product goal and target users
- Architecture decisions
- MVP phases
- Work breakdown
- Acceptance criteria
- Dependencies
- Risks and open questions
- Senior Dev prompt

### Senior Dev

Use `prompts/senior-dev.md`.

Codex should produce:
- Execution plan
- Implementation summary
- Changed areas
- Test summary
- Task repo folder
- PR URL or branch reference
- Docs or artifacts updated
- Open questions or limitations

### Reviewer

Use `prompts/reviewer.md`.

Codex should produce:
- Verdict
- Findings
- Seshat architecture alignment check
- Risks remaining
- Required changes
- Recommendation for next stage

### Hard Task Handoff

Use `prompts/hard-task-handoff.md`.

Use this only when the user explicitly asks for a handoff package or when the task must be split for another executor.

Codex still owns the current requested output unless the user says otherwise.

## Repository Rules

- This repo is the planning and coordination workspace for Seshat.
- If work requires cloning an implementation repository, clone it into `repos/` rather than outside the workspace.
- Use one dedicated repo folder per task with the pattern `{gh_project_name}-{vikunja_project_name}-{task_id}` so task-specific worktrees are isolated.
- Never reuse a repo folder that belongs to a different task, even if the project is the same.
- Use a separate git branch for each task.
- Do not reuse a branch that already contains in-progress work for another task.
- Keep docs and prompts synchronized when the workflow changes.

## Seshat Planning Standards

Planning output should be concrete enough for implementation:

- Define the target layer: CLI, ingestion API, graph schema, query engine, context builder, MCP gateway, storage, deployment, or docs.
- Name the MVP phase and language scope.
- Include input/output contracts when a task touches CLI, server API, graph payload, or MCP tools.
- Include validation expectations: unit tests, golden parser tests, API tests, integration tests, or manual MCP query checks.
- Call out schema compatibility and migration concerns.
- Call out security boundaries: project isolation, auth token handling, private repo metadata, and result limits.

## Difficult Tasks

- Codex should attempt the requested workflow output directly.
- If the work must be split, prepare a clear handoff package and call out what remains unresolved.
- Do not claim the task is review-ready until the required Senior Dev artifacts exist and the implementation result has been validated for the current task.

## Output Discipline

- Prefer the next correct workflow action, not just isolated code changes.
- Include missing artifacts, blockers, or handoff gaps when they affect stage movement.
- Keep responses explicit and easy to copy into Vikunja comments, PR descriptions, or handoff notes.
