# Seshat Vikunja Agent Workflow

Tai lieu nay mo ta quy trinh agent cho du an Seshat.

Seshat la Code Knowledge Graph + AI Context Engine. Data goc va dinh huong kien truc hien tai nam trong `seshat_project_proposal.md`.

## Muc tieu

- Dung Vikunja de dieu phoi discovery, planning, implementation, review, va owner handoff.
- PM + Senior AI phai khao sat proposal va tao plan ro truoc khi task sang dev.
- Moi stage phai de lai artifact co the dung duoc, khong chi de lai comment chung chung.
- Dung labels de biet task dang o dau trong AI workflow.
- Dung buckets de phan anh stage tren board.

## Product Baseline

Neu task khong ghi khac, mac dinh theo cac quyet dinh sau:

- Local CLI scan source code o may dev hoac CI.
- CLI parse code, extract symbol/relation, normalize payload, upload snapshot/delta.
- Server Go xu ly ingestion, validation, graph merge, query engine, context builder, va MCP gateway.
- Storage MVP: PostgreSQL, Redis, object storage.
- Parser adapter theo ngon ngu, output chung graph schema.
- Uu tien language: Ruby + Go truoc, TypeScript/JavaScript sau, Java sau cung.
- MCP gateway la multi-project gateway, moi tool call phai co `project_id`.
- Khong spawn MCP process rieng cho tung project trong MVP neu khong co yeu cau moi.

## Labels Dieu Phoi

- `ai:handled`: task tham gia flow AI.
- `ai:triage`: can PM/Senior AI khao sat, lam ro, hoac lap plan.
- `ai:dev-ready`: da du artifact de Senior Dev implement.
- `ai:dev-running`: dang duoc implement.
- `ai:review-ready`: da co output de review.
- `ai:blocked`: dang bi chan.
- `ai:owner-ready`: da san sang cho owner review.

## Buckets Mac Dinh

- `Inbox`
- `Clarifying`
- `ReadyForDev`
- `InDev`
- `InReview`
- `ReadyForOwner`
- `Done`

## Stage Order

1. Task moi vao flow phai co `ai:handled` va thuong bat dau voi `ai:triage`.
2. PM/Senior AI chi duoc chuyen sang `ReadyForDev` khi da co planning artifact day du.
3. Senior Dev chi duoc set `ai:review-ready` khi implementation hoac artifact da duoc validate.
4. Reviewer chi duoc dua sang `ReadyForOwner` khi khong con blocker va output dat acceptance criteria.
5. Chi owner moi duoc keo `Done`, tru khi owner uy quyen ro rang cho agent mark done.

## PM / Senior AI Output

Dung khi task o `ai:triage`, dac biet luc 1 PM va Senior AI khao sat du an.

Bat buoc co:

- Project understanding
- User/problem statement
- Architecture baseline
- Scope in
- Scope out
- MVP phase mapping
- Work breakdown
- Acceptance criteria
- Dependencies
- Risks
- Open questions
- Senior Dev prompt

Planning phai bam vao `seshat_project_proposal.md`. Neu proposal thieu thong tin, ghi ro missing info va de xuat clarify thay vi doan.

## Senior Dev Output

Bat buoc co:

- Execution plan
- Implementation summary
- Files/services bi anh huong
- Test summary
- Task repo folder neu co tach repo de thuc hien
- PR/link branch hoac artifact thay the
- Docs/artifacts da cap nhat
- Open questions/blockers

Voi Seshat, Senior Dev phai neu ro layer bi anh huong:

- CLI
- ingestion API
- graph schema
- parser adapter
- graph merge/index
- query engine
- context builder
- MCP gateway
- storage
- deployment
- docs/prompts

## Reviewer Output

Bat buoc co:

- Verdict: approved / changes requested / blocked
- Findings
- Architecture alignment check
- Acceptance criteria check
- Test/validation gaps
- Risks con lai
- Next action

Reviewer phai check cac quyet dinh Seshat quan trong:

- Tool/API co `project_id` neu query theo project.
- CLI khong lam phan derived graph ma server can own.
- Parser adapter output theo schema chung.
- Query/result co limit de tranh MCP tra qua nhieu data.
- Project isolation, auth, schema version, commit/version metadata khong bi bo qua neu task lien quan.

## Rule Chuyen Stage

### `ai:triage` -> `ai:dev-ready`

Chi hop le khi comment PM/Senior AI da co:

- context tom tat tu proposal/task
- scope in/out
- acceptance criteria
- dependencies va risks
- open questions neu con
- implementation prompt ro cho Senior Dev
- MVP phase va affected layer

### `ai:dev-running` -> `ai:review-ready`

Chi hop le khi comment Senior Dev da co:

- implementation summary hoac artifact summary
- test summary
- PR/branch/artifact reference
- docs/cach van hanh neu can
- limitations hoac blocker con lai

### `ai:review-ready` -> `ai:owner-ready`

Chi hop le khi review da co:

- verdict approved
- findings da xu ly hoac khong co blocker
- residual risk ro rang
- recommendation cho owner

## Task Kho Dung Codex Hoac Cursor

Neu task kho, cho phep dung Codex hoac Cursor de implement, nhung bat buoc:

1. PM/Senior AI van phai tao Senior Dev prompt va ghi len task.
2. Senior Dev van chiu trach nhiem tong hop context truoc khi handoff.
3. Handoff comment phai neu ro:
   - ly do handoff
   - pham vi duoc giao
   - expected output
   - constraints
4. Sau khi assistant tra output, Senior Dev phai validate va tong hop lai vao task.
5. Khong dua task sang `InReview` neu output chua duoc Senior Dev xac nhan.

## Planning Checklist Cho Seshat

Dung checklist nay khi tao plan du an hoac tach epic/task:

- MVP 1: CLI full scan, Go + Ruby parser, snapshot upload, server luu symbol/relation, query symbol/callers/callees, MCP basic tools.
- MVP 2: JS/TS parser, incremental scan, package dependency graph, impact analysis, `build_llm_context`.
- MVP 3: Java parser, route tracing, event flow, semantic query/reranking.
- Data contracts: CLI config, metadata payload, graph node/edge payload, ingestion API, MCP schemas.
- Storage: projects, project_versions, files, symbols, relations, context_cache.
- Risk control: golden tests per parser, gzip/chunk/delta upload, DB indexes/cache, MCP pagination/max depth/max nodes.

## Nguyen Tac Van Hanh

- Khong nhay coc stage.
- Moi handoff deu phai co comment/artifact tren task.
- Neu thieu thong tin, uu tien `ai:blocked` hoac quay lai `Clarifying`, khong doan.
- Neu task qua lon, tach subtasks theo layer hoac MVP phase.
- Neu can lam viec voi code repository, clone vao `repos/` va dung mot folder repo rieng cho moi task theo pattern `{gh_project_name}-{vikunja_project_name}-{task_id}`.
- Moi task phai dung branch rieng; khong tai su dung branch dang co thay doi cua task khac.
- Khi workflow/prompt thay doi, cap nhat dong bo `docs/`, `prompts/`, va `.agents/skills/vikunja-flow/SKILL.md`.
