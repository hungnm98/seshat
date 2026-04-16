# Seshat Project Plan

Source: `seshat_project_proposal.md`

Role: PM + Senior AI project discovery and planning.

## Project Understanding

Seshat is a Code Knowledge Graph + AI Context Engine. It helps AI agents understand large codebases by indexing source code into symbols, relations, dependency paths, and LLM-ready context blocks.

The practical product shape is:

- A local/CI CLI scans repositories.
- The CLI parses source code and uploads normalized raw analysis payloads.
- A Go server validates, stores, merges, indexes, and serves graph data.
- A query engine answers code intelligence questions.
- A context builder turns graph results into compact LLM context.
- A multi-project MCP gateway exposes project-scoped tools to AI agents.

## Product Goal

Give AI agents reliable, project-scoped code context without requiring the agent to repeatedly scan the full repository or mix context across projects.

Seshat should make these workflows faster and safer:

- Find symbols and definitions.
- Understand callers, callees, imports, and references.
- Estimate impact of a code change.
- Build concise context for an LLM before implementation or review.
- Support private repositories while keeping source scanning under user or CI control.

## Target Users

- Developers using AI coding agents on medium or large repositories.
- Teams that want reusable code intelligence across CI, local machines, and MCP clients.
- AI workflow owners who need project isolation, repeatable indexing, and controlled context size.

## Architecture Decisions

- Use one MCP gateway with project-scoped tools, not one MCP server process per project for MVP.
- Require `project_id` in every project-scoped MCP tool.
- Keep parsing in the CLI or CI runner.
- Keep graph validation, merge, derived graph, indexes, and context cache on the server.
- Use language-specific parser adapters that output one shared graph schema.
- Use PostgreSQL for metadata and relation graph in MVP.
- Use Redis for cache.
- Use object storage for raw snapshot/delta payloads.
- Implement server components in Go.

## System Components

| Component | Responsibility |
| --- | --- |
| CLI | Scan repo, parse source, extract symbols/relations, normalize payload, upload snapshot/delta. |
| Ingestion API | Receive payloads, validate schema/version/auth, enqueue merge/index work. |
| Storage | Store project metadata, versions, files, symbols, relations, raw payloads, and context cache. |
| Graph Builder | Merge payloads, deduplicate by version/file/symbol, build derived graph and indexes. |
| Query Engine | Find symbols, details, callers, callees, dependencies, and impact paths. |
| Context Builder | Rank and compress graph results into LLM-ready context blocks. |
| MCP Gateway | Expose project-scoped tools with auth, quotas, timeouts, and result limits. |

## MVP Phases

### MVP 1: Core Index + Query

Goal: prove the end-to-end path from local scan to MCP query.

Scope:

- CLI full scan.
- Go and Ruby parser adapters.
- Snapshot upload.
- Server ingestion and schema validation.
- PostgreSQL tables for projects, versions, files, symbols, and relations.
- Query APIs for symbol lookup and callers/callees.
- MCP tools:
  - `find_symbol`
  - `get_symbol_detail`
  - `find_callers`
  - `find_callees`

Acceptance criteria:

- A sample Go repo and sample Ruby repo can be scanned.
- Uploaded payloads include project/version metadata.
- Server rejects invalid schema versions or missing required fields.
- Symbol and relation records can be queried by `project_id`.
- MCP tool responses are bounded and project-isolated.
- Golden parser tests cover representative Go and Ruby symbols/relations.

### MVP 2: Incremental + Context

Goal: make indexing efficient and useful for AI implementation workflows.

Scope:

- TypeScript and JavaScript parser adapters.
- Incremental scan by changed file/commit.
- Delta upload.
- Package/module dependency graph.
- Basic impact analysis.
- Context builder.
- MCP tool:
  - `impact_analysis`
  - `build_llm_context`

Acceptance criteria:

- CLI can send delta payloads for changed files.
- Server invalidates affected graph/cache records.
- Impact analysis respects max depth/max nodes.
- Context builder returns ranked blocks with max block limits.
- JS/TS golden tests cover imports, functions, classes, and references.

### MVP 3: Advanced Flow Tracing

Goal: support deeper codebase reasoning across application patterns.

Scope:

- Java parser adapter.
- Route tracing.
- Event flow.
- Semantic query and reranking.
- Relations such as `route_to_handler`, `handler_to_service`, `reads_from`, `writes_to`, `emits_event`, and `consumes_event`.

Acceptance criteria:

- Java parser emits compatible graph payloads.
- Route tracing can start from configured entrypoints.
- Semantic query still enforces project isolation and result limits.
- Advanced relations are versioned and documented.

## Work Breakdown

## Current Repository Baseline

The repository already contains a bootstrap implementation for several MVP 1 and early MVP 2 foundations. Treat these as existing assets to extend, not as work to recreate:

- Runtime commands exist under `cli/cmd/seshat`, `server/cmd/server`, `server/cmd/worker`, and `server/cmd/mcp`.
- Admin auth/dashboard and project token flow exist under `server/internal/admin`, `server/internal/api`, and `server/internal/auth`.
- CLI ingestion and project-scoped bearer token flow are present.
- Go and Ruby parser foundations exist under `cli/internal/parser/golang` and `cli/internal/parser/ruby`.
- Shared graph schema exists under `cli/pkg/graphschema` and `server/pkg/graphschema`.
- Query service exists under `server/internal/query`.
- Memory and PostgreSQL storage adapters exist under `server/internal/storage`.
- Redis and object storage wrappers exist under `server/internal/storage/redis` and `server/internal/storage/objectstore`.
- Worker follow-up plumbing exists under `server/cmd/worker`.
- Incremental Go-first scan behavior is represented in the CLI/parser layer.
- Roadmap/task artifacts exist under `tasks/`.

Planning and new Vikunja tasks should reference this baseline before proposing new scaffolding.

### 1. Contracts And Schema

- Audit existing CLI config, graphschema v1, ingestion, query, and MCP contracts.
- Add missing docs for CLI config, analysis metadata payload, graph node/edge schema, and MCP tools.
- Confirm relation type enum for MVP 1 matches implementation and proposal.
- Document ingestion API request/response and error model.
- Document schema compatibility expectations for future CLI versions.

Deliverables:

- `docs/contracts/cli-config.md`
- `docs/contracts/analysis-payload.md`
- `docs/contracts/mcp-tools.md`

### 2. CLI

- Preserve existing scan command behavior.
- Tighten include/exclude path handling where needed.
- Confirm language target detection against Go/Ruby fixtures.
- Confirm commit, branch, schema_version, and generated timestamp are present.
- Extend incremental scan beyond current Go-first behavior when entering MVP 2.
- Add upload behavior checks for snapshot and later delta modes.

Validation:

- CLI config tests.
- Payload schema tests.
- Snapshot upload integration test with a local test server.

### 3. Parser Adapters

- Extend existing Go and Ruby adapters.
- Normalize all adapter output into shared graphschema v1.
- Improve relation fidelity for MVP relation types.
- Add or expand golden fixtures per language.
- Add TypeScript/JavaScript adapters in MVP 2.
- Add Java adapter in MVP 3.

Validation:

- Golden tests for `declared_in`, `imports`, `calls`, `references`, and `contains`.
- Fixture repos for simple package/module layouts.

### 4. Server Ingestion

- Preserve existing project token and ingestion flow.
- Validate auth/project access.
- Validate schema version and required metadata.
- Store raw payload through the object storage abstraction when configured.
- Store project/version/file/symbol/relation records through the selected store.
- Move follow-up merge/index/cache work into worker lanes where appropriate.

Validation:

- API tests for valid payloads.
- API tests for missing `project_id`, invalid `schema_version`, duplicate payload, and unsupported language.

### 5. Query Engine

- Extend existing query service.
- Confirm symbol search, symbol detail, callers, and callees contracts.
- Enforce depth and result limits.
- Add or verify indexes for common PostgreSQL query paths.
- Add impact analysis in MVP 2.

Validation:

- Query tests by `project_id`.
- Depth and result limit tests.
- Cross-project isolation tests.

### 6. MCP Gateway

- Align existing `server/cmd/mcp` contract with MVP 1 tools.
- Require `project_id` for project-scoped calls.
- Enforce auth, quota, timeout, and max result limits.
- Return compact structured responses suitable for AI agents.

Validation:

- Tool schema tests.
- Missing `project_id` rejection.
- Large result truncation behavior.
- Manual MCP query smoke test.

### 7. Context Builder

- Build in MVP 2 after query primitives are stable.
- Rank graph results for a user question.
- Produce bounded context blocks.
- Cache context by project/query/version hash.

Validation:

- Max blocks and truncation tests.
- Cache invalidation tests after delta upload.

## Data Contracts To Define First

### CLI Config

Required fields:

- `project_id`
- `repo_path`
- `language_targets`
- `include_paths`
- `exclude_paths`
- `scan_mode`

### Analysis Metadata

Required fields:

- `project_id`
- `commit_sha`
- `branch`
- `schema_version`
- `generated_at`

### Node

Minimum fields:

- `id`
- `kind`
- `name`
- `language`
- `file`
- `line_start`
- `line_end`

### Edge

Minimum fields:

- `from`
- `to`
- `type`
- `metadata`

### MVP Relation Types

- `declared_in`
- `imports`
- `calls`
- `implements`
- `references`
- `contains`

## Initial Task List

## Next Task List

1. Add contract docs for CLI config, analysis payload, graph nodes/edges, ingestion API, and MCP tools based on the current code.
2. Reconcile `tasks/boards/roadmap.md` with the completed TASK-001 through TASK-009 baseline and define the next active task set.
3. Complete or validate TASK-010 end-to-end HTTP integration coverage for admin and ingestion flows.
4. Harden MCP MVP 1 tool contracts and ensure every project-scoped path requires `project_id`.
5. Expand Go/Ruby parser golden fixtures for `declared_in`, `imports`, `calls`, `references`, and `contains`.
6. Document PostgreSQL migration and store-selection behavior for local and deployment environments.
7. Add result limit/depth/error-path tests for query and MCP paths.
8. Define MVP 2 incremental delta payload contract before expanding beyond Go-first incremental scan.
9. Prepare TypeScript/JavaScript parser adapter design for MVP 2.
10. Define context builder ranking, truncation, cache key, and invalidation contract.

## Risks

- Parser adapters may emit inconsistent symbols and relation semantics.
- Payloads may become too large for direct upload.
- Recursive graph queries may become slow without indexes and result limits.
- MCP tools may return too much context if limits are not enforced early.
- Private repository metadata may leak if project isolation/auth is weak.
- Schema changes may break older CLI versions if versioning is not explicit.

## Risk Controls

- Golden tests per language adapter.
- Strict graph schema and relation type docs.
- `schema_version` in every payload.
- Gzip/chunk upload for large snapshots.
- Delta upload in MVP 2.
- PostgreSQL indexes for project/version/relation query paths.
- Redis cache for hot queries and context blocks.
- Max depth, max nodes, pagination, timeout, and quota on query/MCP paths.
- Auth checks at ingestion and MCP gateway.

## Open Questions

- Is the current project-scoped bearer token model sufficient for MVP, or should organization/user-scoped tokens be introduced before multi-user rollout?
- Should project IDs be user-provided strings, server-generated IDs, or both?
- Should local object storage default to MinIO only, or should filesystem-backed raw payload storage remain available for simple development?
- Should Ruby parsing stay heuristic for MVP 1, or move to Prism before expanding relation fidelity?
- Should the first CLI be distributed as a Go binary even for Ruby parser support, or should parser execution support external adapters?
- What is the expected maximum repository size for MVP validation?

## Senior Dev Handoff Prompt

```text
Implement Seshat MVP 1 foundation.

Use seshat_project_proposal.md and docs/seshat-project-plan.md as the source of truth.

Scope:
- Define contracts for CLI config, analysis payload, graph nodes/edges, and MVP MCP tools.
- Scaffold the Go server around ingestion, storage, query, context, and MCP gateway packages.
- Scaffold the CLI around scan config, language target selection, parser adapter execution, and snapshot upload.
- Implement the initial PostgreSQL schema for projects, project_versions, files, symbols, relations, and context_cache.
- Keep parser adapters behind a shared interface and start with Go + Ruby.
- Implement MVP 1 query/MCP contract shape for find_symbol, get_symbol_detail, find_callers, and find_callees.

Architecture constraints:
- CLI should emit normalized raw analysis payloads only.
- Server owns validation, graph merge, derived graph, indexes, and context cache.
- Every project-scoped MCP/query path must require project_id.
- Use one multi-project MCP gateway, not per-project MCP processes.
- Preserve project_id, commit_sha, branch, schema_version, and generated_at metadata.

Validation:
- Add schema validation tests.
- Add parser golden test structure.
- Add query tests for project isolation and result limits.
- Document any unimplemented parts as explicit TODOs tied to MVP phase.
```
