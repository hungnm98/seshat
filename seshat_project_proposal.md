# Đề xuất dự án Code Knowledge Graph + AI Context Engine

## 1. Đánh giá nhanh giải pháp bạn đề xuất

Giải pháp:

- User dùng **CLI** để quét source code và tạo dữ liệu phân tích tại local.
- CLI đẩy dữ liệu đã chuẩn hóa lên **server storage**.
- Server lưu graph, index, metadata, context cache.
- Tạo thêm **MCP theo từng project** để AI agent query đúng project.

### Kết luận ngắn
**Có, hướng này ổn và thực tế.** Đây là một thiết kế tốt cho giai đoạn MVP và vẫn mở rộng được về sau.

### Vì sao ổn
- **Giảm tải cho server**: parse code, đọc AST, detect symbol, build raw relation ngay ở máy user hoặc CI runner.
- **Dễ scale**: server tập trung vào lưu trữ, query, cache, context building, MCP serving.
- **Bảo mật hơn**: user có thể kiểm soát repo private tại local/CI; chỉ đẩy metadata hoặc graph snapshot đã chọn.
- **Hỗ trợ incremental tốt**: CLI chỉ cần gửi delta theo commit hoặc theo file thay đổi.
- **Phù hợp đa ngôn ngữ**: parser nằm ở CLI/plugin layer, dễ mở rộng cho Ruby/Go/Java/JS/TS.
- **Tốt cho AI agent**: MCP theo project giúp context chính xác, giảm lẫn dữ liệu giữa nhiều repo.

### Điểm cần lưu ý
- Nếu mỗi project có một MCP server riêng thì số lượng process có thể tăng mạnh khi nhiều project.
- Cần chuẩn hóa **schema node/edge** thật chặt, nếu không parser mỗi ngôn ngữ sẽ đẩy dữ liệu không đồng nhất.
- Cần hỗ trợ **project isolation**, auth token, versioning schema, dedup theo commit hash.
- Nên phân biệt rõ:
  - **raw index data**,
  - **derived graph**,
  - **LLM context cache**.

### Khuyến nghị kiến trúc
Thay vì chạy một process MCP hoàn toàn riêng cho từng project, nên dùng:

- **1 MCP Gateway**
- mỗi tool call bắt buộc có `project_id`
- phía sau query engine sẽ route theo project

Mô hình này dễ vận hành hơn nhiều so với việc spawn một MCP server cho từng repo.

Chỉ nên tạo **MCP namespace theo project**, không nhất thiết tạo **server process riêng per project**.

---

## 2. Ưu tiên ngôn ngữ phân tích

Bạn muốn ưu tiên các ngôn ngữ:

1. **Ruby**
2. **Go**
3. **Java**
4. **JavaScript**
5. **TypeScript**

### Thứ tự triển khai đề xuất
Nên triển khai parser theo thứ tự sau:

#### Phase A
- Go
- Ruby

#### Phase B
- TypeScript
- JavaScript

#### Phase C
- Java

### Lý do
- **Go**: dễ parse, AST rõ, hiệu năng tốt, phù hợp làm chuẩn cho graph model đầu tiên.
- **Ruby**: rất quan trọng nếu bạn có nhiều codebase Rails/backend Ruby.
- **TypeScript/JavaScript**: phổ biến cho frontend + Node.js, cần support sớm.
- **Java**: enterprise-heavy, nhưng parser và semantic resolution thường phức tạp hơn, nên đưa sau khi graph model đã ổn.

---

## 3. Đề xuất tên project theo thần cổ đại

Dưới đây là các tên phù hợp cho một hệ thống phân tích code, dựng graph, phục vụ AI.

## Nhóm tên mạnh về tri thức / ghi chép / hiểu biết

### 1. Thoth
- Nguồn gốc: thần Ai Cập cổ đại
- Ý nghĩa: tri thức, chữ viết, ghi chép, ma thuật
- Rất hợp với hệ thống đọc và hiểu codebase

**Đánh giá:** rất mạnh cho branding kỹ thuật

### 2. Odin
- Nguồn gốc: Bắc Âu
- Ý nghĩa: trí tuệ, khám phá, hy sinh để đạt tri thức
- Hợp cho AI engine hiểu code sâu

**Đánh giá:** mạnh, dễ nhớ, hơi phổ biến

### 3. Athena
- Nguồn gốc: Hy Lạp
- Ý nghĩa: trí tuệ, chiến lược, thủ công
- Hợp nếu muốn tên tinh tế hơn

**Đánh giá:** đẹp, dễ branding

### 4. Seshat
- Nguồn gốc: Ai Cập
- Ý nghĩa: thần ghi chép, thư viện, tri thức
- Rất hợp với code indexing / code knowledge

**Đánh giá:** độc đáo, ít bị trùng

### 5. Mimir
- Nguồn gốc: Bắc Âu
- Ý nghĩa: tri thức, giếng trí tuệ
- Hợp cho knowledge engine

**Đánh giá:** ngắn, đẹp, technical

---

## Nhóm tên mạnh về dẫn đường / kết nối / trung gian

### 6. Hermes
- Nguồn gốc: Hy Lạp
- Ý nghĩa: sứ giả, trung gian, truyền tin
- Hợp cho MCP/tool-calling gateway

**Đánh giá:** rất hợp nếu nhấn vào AI tool routing

### 7. Iris
- Nguồn gốc: Hy Lạp
- Ý nghĩa: sứ giả kết nối trời và đất
- Hợp cho context bridge giữa codebase và AI

**Đánh giá:** mềm hơn, đẹp

### 8. Heimdall
- Nguồn gốc: Bắc Âu
- Ý nghĩa: người gác cổng, quan sát tất cả
- Hợp cho project query gateway, graph observability

**Đánh giá:** rất ngầu, hợp hạ tầng

---

## Nhóm tên mạnh về bản đồ / cấu trúc / hệ thống

### 9. Janus
- Nguồn gốc: La Mã
- Ý nghĩa: nhìn hai phía, cổng chuyển tiếp
- Hợp cho hệ thống hiểu quan hệ giữa modules và flows

### 10. Atlas
- Nguồn gốc: Hy Lạp
- Ý nghĩa: mang cả thế giới, bản đồ
- Hợp cho code graph và system map

### 11. Anansi
- Nguồn gốc: Tây Phi
- Ý nghĩa: mạng lưới, trí khôn, kể chuyện
- Hợp nếu muốn hình ảnh “web of code”

---

## 4. Tên nên chọn

### Top 5 khuyến nghị
1. **Thoth**
2. **Seshat**
3. **Mimir**
4. **Hermes**
5. **Atlas**

### Gợi ý chọn theo định vị

#### Nếu muốn nhấn mạnh “AI hiểu code”
- **Thoth**
- **Mimir**

#### Nếu muốn nhấn mạnh “code graph / knowledge graph”
- **Seshat**
- **Atlas**

#### Nếu muốn nhấn mạnh “MCP / tool routing / AI bridge”
- **Hermes**
- **Heimdall**

### Tên mình khuyên chọn nhất
## **Thoth**

Vì:
- ngắn
- dễ nhớ
- rất hợp semantic “tri thức, ghi chép, hiểu biết”
- phù hợp với sản phẩm đọc code, build graph, cấp context cho AI

---

## 5. Kiến trúc đề xuất đã chỉnh theo ý bạn

## 5.1 Thành phần chính

### A. Local CLI
Chạy ở máy dev hoặc CI.

Nhiệm vụ:
- scan repo
- parse source code
- trích xuất symbol
- build raw relation
- chuẩn hóa node/edge
- gửi snapshot hoặc delta lên server

### B. Storage Server
Nhiệm vụ:
- nhận dữ liệu từ CLI
- lưu metadata project
- lưu node/edge graph
- lưu commit/version
- lưu query index
- lưu context cache

### C. Query Engine
Nhiệm vụ:
- tìm symbol
- tìm caller/callee
- query dependency
- impact analysis
- route tracing
- package/module summary

### D. Context Builder
Nhiệm vụ:
- gom dữ liệu từ graph
- rút gọn thành context phù hợp cho LLM
- rank các context block theo câu hỏi

### E. MCP Gateway
Nhiệm vụ:
- expose tool cho AI agent
- route query theo `project_id`
- enforce auth / quota / timeout / max result

---

## 5.2 Luồng vận hành

### Luồng 1: full scan
1. User chạy CLI trong repo
2. CLI parse toàn bộ project
3. CLI tạo graph snapshot
4. CLI gửi snapshot lên server
5. Server validate schema
6. Server upsert graph
7. Query engine build index
8. MCP có thể query ngay theo project

### Luồng 2: incremental scan
1. User pull code mới hoặc có commit mới
2. CLI detect file changed
3. Chỉ parse file/package ảnh hưởng
4. CLI gửi delta
5. Server cập nhật node/edge bị ảnh hưởng
6. Cache liên quan bị invalidation

### Luồng 3: AI query qua MCP
1. User hỏi AI agent
2. Agent gọi MCP tool với `project_id`
3. MCP gateway gọi query engine
4. Query engine trả dữ liệu graph
5. Context builder tạo summary/context ngắn
6. AI dùng context để trả lời

---

## 6. Input / Output cần chốt rõ

## 6.1 Input của CLI

### Input bắt buộc
- `project_id`
- `repo_path`
- `language_targets`
- `include_paths`
- `exclude_paths`
- `scan_mode`

### Ví dụ
```json
{
  "project_id": "thoth-payment-service",
  "repo_path": "/workspace/payment-service",
  "language_targets": ["ruby", "go", "java", "javascript", "typescript"],
  "include_paths": ["app", "lib", "internal", "pkg", "src"],
  "exclude_paths": ["vendor", "node_modules", "dist", "build", "tmp", "coverage"],
  "scan_mode": "incremental"
}
```

## 6.2 Output của CLI gửi lên server

### Dữ liệu metadata
```json
{
  "project_id": "thoth-payment-service",
  "commit_sha": "abc123",
  "branch": "main",
  "schema_version": "v1",
  "generated_at": "2026-04-02T10:00:00Z"
}
```

### Dữ liệu graph mẫu
```json
{
  "nodes": [
    {
      "id": "symbol:create_order",
      "kind": "method",
      "name": "CreateOrder",
      "language": "go",
      "file": "internal/order/service.go",
      "line_start": 18,
      "line_end": 80
    }
  ],
  "edges": [
    {
      "from": "symbol:create_order_handler",
      "to": "symbol:create_order",
      "type": "calls"
    }
  ]
}
```

---

## 7. Thiết kế MCP/tool calling theo project

## 7.1 Nguyên tắc
Mọi tool phải nhận `project_id`.

### Ví dụ
- `find_symbol(project_id, query, kind?)`
- `get_symbol_detail(project_id, symbol_id)`
- `find_callers(project_id, symbol_id, depth?)`
- `find_callees(project_id, symbol_id, depth?)`
- `impact_analysis(project_id, symbol_id, max_depth?)`
- `trace_request_flow(project_id, entrypoint)`
- `build_llm_context(project_id, question, max_blocks?)`

## 7.2 Ví dụ tool schema

### Tool: `find_symbol`
```json
{
  "name": "find_symbol",
  "description": "Find symbols in a project by name or fuzzy query",
  "input_schema": {
    "type": "object",
    "properties": {
      "project_id": { "type": "string" },
      "query": { "type": "string" },
      "kind": { "type": "string" }
    },
    "required": ["project_id", "query"]
  }
}
```

### Tool response
```json
{
  "results": [
    {
      "symbol_id": "symbol:create_order",
      "name": "CreateOrder",
      "kind": "method",
      "language": "go",
      "file": "internal/order/service.go",
      "line_start": 18,
      "line_end": 80
    }
  ]
}
```

---

## 8. Stack công nghệ khuyến nghị

## 8.1 Thành phần server
- **Go** cho:
  - API server
  - ingestion endpoint
  - query engine
  - MCP gateway
  - context builder
  - worker

## 8.2 Parser
- **Go native parser** cho Go
- **Ruby Prism** hoặc parser gem phù hợp cho Ruby
- **tree-sitter-java** cho Java
- **tree-sitter-javascript**
- **tree-sitter-typescript**

### Lý do
- Không nên cố ép mọi ngôn ngữ dùng một parser duy nhất.
- Nên có **adapter per language**, output cùng một schema graph chung.

## 8.3 Storage
### Phương án chính
- **PostgreSQL** cho metadata + relation table + recursive query
- **Redis** cho cache
- **Object Storage** cho raw snapshot/delta

### Phương án thay thế
- **Neo4j** nếu bạn muốn tối ưu graph traversal sâu và query trực quan

### Khuyến nghị thực tế
MVP nên dùng:
- PostgreSQL
- Redis
- S3/MinIO

Lý do:
- dễ deploy
- dễ backup
- dễ vận hành hơn Neo4j
- phù hợp team backend truyền thống

---

## 9. Thiết kế project structure đề xuất

```text
thoth/
  cmd/
    server/
    cli/
    worker/
    mcp/
  internal/
    api/
    auth/
    config/
    ingestion/
    parser/
      common/
      golang/
      ruby/
      java/
      javascript/
      typescript/
    graph/
    query/
    context/
    storage/
      postgres/
      redis/
      objectstore/
    worker/
    mcp/
    project/
  pkg/
    model/
    graphschema/
    logger/
    errkit/
  deployments/
    docker/
    k8s/
  docs/
  scripts/
```

---

## 10. Giải pháp tối ưu hơn cho CLI

Thay vì để CLI build toàn bộ graph cuối cùng, nên chia 2 lớp:

### CLI làm
- parse source
- normalize symbol + relation raw
- gửi raw analysis payload

### Server làm
- validate
- merge
- dedup
- build derived graph
- build query index
- build context cache

### Lợi ích
- server giữ quyền kiểm soát graph model
- dễ migrate schema
- dễ sửa bug trong graph builder mà không cần update CLI quá thường xuyên

---

## 11. Thiết kế data model rút gọn

## Bảng `projects`
- id
- name
- default_branch
- created_at

## Bảng `project_versions`
- id
- project_id
- commit_sha
- branch
- status
- scanned_at

## Bảng `files`
- id
- project_version_id
- path
- language
- checksum

## Bảng `symbols`
- id
- project_version_id
- file_id
- kind
- name
- signature
- visibility
- line_start
- line_end

## Bảng `relations`
- id
- project_version_id
- from_symbol_id
- to_symbol_id
- relation_type
- metadata_json

## Bảng `context_cache`
- id
- project_id
- query_hash
- context_json
- expires_at

---

## 12. Nên hỗ trợ loại relation nào trước

### MVP relations
- `declared_in`
- `imports`
- `calls`
- `implements`
- `references`
- `contains`

### Phase 2
- `inherits`
- `route_to_handler`
- `handler_to_service`
- `service_to_repository`
- `reads_from`
- `writes_to`

### Phase 3
- `emits_event`
- `consumes_event`
- `transaction_scope`
- `cross_service_call`

---

## 13. Khuyến nghị MVP thực dụng

## MVP 1
- CLI scan local repo
- support Go + Ruby
- upload snapshot lên server
- server lưu symbol/relation
- query symbol
- query callers/callees
- MCP tool: `find_symbol`, `get_symbol_detail`, `find_callers`, `find_callees`

## MVP 2
- thêm JS/TS
- incremental scan
- package dependency graph
- impact analysis cơ bản
- build_llm_context

## MVP 3
- thêm Java
- route tracing
- event flow
- semantic query + reranking

---

## 14. Rủi ro và cách tránh

### Rủi ro 1: graph sai vì parser khác nhau
**Cách tránh:** định nghĩa schema trung gian thật rõ và viết golden test cho từng language adapter.

### Rủi ro 2: payload CLI quá lớn
**Cách tránh:** gzip, chunk upload, delta upload, raw snapshot lưu object storage.

### Rủi ro 3: query chậm khi graph lớn
**Cách tránh:** index relation table, cache hot query, precompute dependency summary.

### Rủi ro 4: MCP trả quá nhiều data cho AI
**Cách tránh:** pagination, max depth, max nodes, context summarization.

---

## 15. Quyết định kiến trúc cuối cùng mình khuyên

## Tên project khuyên dùng
**Thoth**

## Kiến trúc khuyên dùng
- **CLI local/CI-based analysis**
- **Go server for ingestion/query/MCP**
- **PostgreSQL + Redis + Object Storage**
- **MCP gateway dùng `project_id` để route**
- **Parser adapter per language**
- ưu tiên:
  - Ruby
  - Go
  - Java
  - JavaScript
  - TypeScript

## Nhận định cuối
Giải pháp bạn đề xuất **ổn**, thậm chí là hướng khá thực tế để đi MVP nhanh mà vẫn giữ đường nâng cấp lên production.

Điểm mình chỉnh mạnh nhất là:
- không nên chạy process MCP riêng cho từng project ngay từ đầu,
- nên dùng **1 MCP gateway + project-scoped tools**,
- CLI chỉ nên chịu trách nhiệm **analysis + upload raw normalized payload**,
- server chịu trách nhiệm **graph merge + query + context build**.

---

## 16. Prompt ngắn gọn để tiếp tục giao cho AI code

```text
Hãy triển khai hệ thống Thoth - Code Knowledge Graph + AI Context Engine.

Yêu cầu:
- User dùng CLI để scan source code local hoặc trong CI.
- CLI hỗ trợ ưu tiên ngôn ngữ: Ruby, Go, Java, JavaScript, TypeScript.
- CLI parse source code, trích xuất symbol/relation, normalize payload và upload lên server.
- Server viết bằng Go, chịu trách nhiệm ingestion, graph merge, query engine, context builder và MCP gateway.
- Mỗi AI tool call phải có project_id để query đúng project.
- Không tạo MCP process riêng cho từng project ở MVP; dùng một MCP gateway đa project.
- Dùng PostgreSQL lưu metadata và relation graph, Redis để cache, object storage để lưu snapshot/delta.
- Thiết kế input/output rõ ràng cho CLI, ingestion API, query API và MCP tools.
- Ưu tiên hiệu năng, incremental scan, extensibility và production-ready architecture.
```
