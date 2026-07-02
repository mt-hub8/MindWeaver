# AI Knowledge Assistant · AI Task Orchestrator

## 产品定位（第一屏）

**AI Task Orchestrator 是一个面向企业 AI 应用的知识库问答与 RAG 质量调试系统。** 用户可以摄入私有文档，在浏览器中提问，系统基于文档生成带引用的回答，并展示检索策略、引用来源和质量评估结果。

| | |
|---|---|
| **适用场景** | 内部知识库问答、RAG 方案验证、检索策略对比（dense / rerank / hybrid）、演示与面试展示 |
| **核心能力** | **V8.0 真实本地 AI Runtime（Ollama）** · **V7.0 Tool-Using Agent Workflow** · **V6.0 AI 任务编排** · **V5.0 知识库分组** · **V4.0 知识库生命周期** · RAG / Rerank / Hybrid |
| **5 分钟体验** | 启动服务 → Documents 上传 → （可选）Collections 分组 → Ask 提问 / **Agent Tasks 提交任务** → 查看执行步骤与结果 |
| **主要入口** | Web：`/` · `/documents.html` · `/collections.html` · `/ask.html` · **`/agent-tasks.html`** · **`/agent-tools.html`** · API：`GET /agent/tools` · `POST /agent/tasks` · `POST /rag/answers` |

> 技术栈：Java 17 · Spring Boot · MySQL · RabbitMQ · JPA/Flyway · 可选 Qdrant / Local Embedding Worker。下文保留完整能力与架构说明。

---

## 1. 项目定位（技术视角）

AI Task Orchestrator 是一个基于 Java / Spring Boot 的 AI 任务编排与 RAG 检索后端系统。它的重点不是简单调用大模型 API，而是构建面向 LLM / RAG / Agent 工作负载的可靠异步底座，并在其上逐步扩展文档处理、Embedding、向量检索、检索评估与 RAG 问答能力。

英文一句话定位：

> AI Task Orchestrator is a Java / Spring Boot backend for orchestrating long-running AI workloads and building a RAG retrieval foundation, including async dispatch, transactional outbox, atomic task claiming, embedding provider abstraction, vector store abstraction, retrieval evaluation, rerank, and hybrid retrieval fusion.

当前项目是 **production-oriented prototype**，不是完整 production-grade platform，也不是完整 Agent Runtime。

---

## 2. 当前能力总览

### Implemented（已实现）

**Reliable Async Task Execution**

- Task lifecycle / state machine
- RabbitMQ async dispatch
- Transactional Outbox
- Atomic Task Claim
- Task Attempt（`task_attempt` 持久化 + `GET /tasks/{taskId}/attempts`）
- Retry / cancellation / timeout
- Idempotent consumption
- Mock LLM Client
- Prompt Template
- Model Router
- LLM usage metadata
- Persisted output chunks

**Document & Chunking**

- Document Upload（`.txt` / `.md`）
- Fixed Chunking / Adaptive Chunking（`DocumentChunker`）
- chunk metadata：`headingPath`、`startOffset`、`endOffset`、`chunkStrategy`

**Embedding**

- `EmbeddingProvider` 抽象
- Mock Embedding Provider（默认）
- OpenAI-compatible Embedding Provider
- Local Embedding Worker provider integration（Java 侧接入）
- provider / model / dimension metadata 保存与校验

**Retrieval & Evaluation**

- Document embedding generation（`POST /documents/{documentId}/embeddings`）
- Document search TopK（`POST /documents/search`）
- Retrieval Evaluation Harness（`POST /evaluations/retrieval`）
- Recall@K / Precision@K / HitRate@K / MRR / NDCG@K / ContextPrecision@K
- Evaluation Dataset Seed（`retrieval-corpus-v1.md` + `retrieval-benchmark-v1.json`）
- Evidence Mapper（expectedEvidenceIds → expectedChunkIds）
- Fixed vs Adaptive Chunking Comparison（测试 harness）
- Embedding Provider Benchmark Comparison（测试 harness）

**VectorStore**

- `VectorStore` 抽象
- `ExactCosineVectorStore`（默认 baseline）
- `QdrantVectorStore`（显式配置时启用）
- VectorStore Benchmark Harness（测试 harness，baseline vs candidate）

**RAG（问答与检索策略）**

- RAG Answer with Citation API（`POST /rag/answers`）
- Two-stage Retrieval & Rerank（V2.9，配置 `rag.rerank.enabled`）
- App-layer Hybrid Retrieval Fusion（V3.0，配置 `rag.hybrid.enabled`）
- 浏览器产品入口：`/` · `/ask.html` · `/documents.html` · `/evaluation.html`

**Knowledge Base Lifecycle（V4.0）**

- 文档生命周期状态：已启用（ACTIVE）/ 已删除（DELETED）
- 文档软删除：`DELETE /documents/{documentId}`，不物理清理 document / chunks / vectors
- 已删除文档与旧版本片段不再进入 Ask / RAG（应用层过滤，含 hybrid / rerank）
- 文档重新建立索引（重新索引）：`POST /documents/{documentId}/reindex`，复用 `source_text` 异步重建索引
- 当前索引版本（`current_generation`）与 chunk 代际过滤
- 中文文档管理页面、生命周期事件时间线、文档处理分析入口
- 说明文档：[docs/manual/knowledge-base-lifecycle-management.md](docs/manual/knowledge-base-lifecycle-management.md)

**Scoped Retrieval & Collections（V5.0）**

- 知识库分组（Collection）：`POST /collections`、`GET /collections`、`GET /collections/{id}`
- 文档加入 / 移出分组：`POST|DELETE /collections/{collectionId}/documents/{documentId}`
- Ask 范围检索：`POST /rag/answers` 可选 `collectionId`；不传时保持全库检索
- 应用层同时过滤：分组范围 + V4.0 生命周期（已删除文档、旧版本片段）
- 页面：`/collections.html` · Ask / Documents 页面支持分组管理与范围选择
- 说明文档：[docs/manual/scoped-retrieval-and-collections.md](docs/manual/scoped-retrieval-and-collections.md)

**AI Runtime & Agent Task Orchestration（V6.0）**

- Java 主编排 + Python AI Runtime Worker（embedding / LLM，`local-ai` profile）
- 默认测试与默认 profile 使用 **mock** provider，不依赖真实模型
- AI 任务编排：`POST /agent/tasks` → RabbitMQ 异步执行 → 检索 + LLM → 结果 / 引用 / 事件时间线
- 页面：`/agent-tasks.html`
- 说明文档：[docs/manual/ai-runtime-and-agent-task-orchestration.md](docs/manual/ai-runtime-and-agent-task-orchestration.md)

**Tool-Using Agent Workflow（V7.0）**

- 固定三步工具流程：检索知识库 → 总结检索结果 → 生成最终报告
- 内置工具注册表：`GET /agent/tools`（`knowledge_search`、`context_summary`、`collection_overview`）
- 任务详情含执行步骤、工具输入/输出、引用来源与事件时间线
- 页面：`/agent-tasks.html` · `/agent-tools.html`
- 说明文档：[docs/manual/tool-using-agent-workflow.md](docs/manual/tool-using-agent-workflow.md)

**Real Local AI Runtime（V8.0）**

- 免费本地体验：**Ollama** + `qwen3-embedding:0.6b` + `qwen2.5:7b`（或 `qwen2.5:3b` 备用）
- 链路：Java → Python Worker → Ollama；**默认 profile 仍为 mock**
- 启用方式：`local-ai` profile + 手工启动 Python Worker 与 Ollama
- 说明文档：[docs/manual/real-local-ai-runtime-with-ollama.md](docs/manual/real-local-ai-runtime-with-ollama.md)

**工程化**

- Flyway migration
- Docker Compose（MySQL / RabbitMQ）
- GitHub Actions CI

### Prototype / Experimental（原型或实验性实现）

以下能力代码已存在，但不应表述为 production-ready：

- **Local Embedding Worker**：Python FastAPI + sentence-transformers，需手工启动，默认测试不依赖
- **QdrantVectorStore**：REST 接入 Qdrant，需手工启动 Qdrant 并配置 `app.vector-store.provider=qdrant`
- **OpenAI-compatible embedding**：需配置 `OPENAI_API_KEY` 与 `app.embedding.provider=openai`，默认测试不调用
- **Exact vs candidate VectorStore benchmark**：通过 `VectorStoreBenchmarkComparisonTest` 等测试验证，不进入默认 CI 外部依赖
- **Embedding Provider benchmark comparison**：通过测试 harness 对比 mock / fake candidate provider
- **RAG Answer**：链路已打通，但 LLM 仍为 Mock，未做 Generation Evaluation

### Not Implemented Yet（尚未实现）

- Production-grade RAG answer / generation quality governance
- Auth / tenant / quota
- API rate limit
- Production observability dashboard（完整 metrics pipeline）
- Distributed worker registry
- Agent Runtime
- KV Cache-aware scheduling
- Production-grade Qdrant deployment（Docker Compose 集成、健康检查、模型缓存治理）
- Real billing / subscription system
- Evaluation result persistence
- Generation Evaluation（Faithfulness、LLM-as-a-judge 等）
- Web UI 登录 / 权限 / 文档内容在线编辑

---

## 3. 系统架构概览

**任务编排链路**

```text
HTTP API
-> TaskService / DocumentService
-> MySQL（task / task_event / task_outbox / task_attempt / document / document_chunk / document_chunk_embedding）
-> Outbox Dispatcher
-> RabbitMQ
-> Consumer
-> Atomic Task Claim
-> TaskExecutionService
-> Prompt Template / ModelRouter / LlmClient
-> task_attempt / task_output_chunk 更新
```

**RAG 检索链路**

```text
Document Upload
-> Adaptive Chunking
-> EmbeddingProvider（mock / openai / local-worker）
-> VectorStore（exact / qdrant）
-> Document Search TopK
-> RetrievalEvaluation / RAG Answer
```

两层配置分离：

- `app.embedding.provider`：文本如何生成向量
- `app.vector-store.provider`：向量如何存储与检索

---

## 4. 核心链路一：Reliable Async Task Execution

```text
POST /tasks
-> task 入库，status = PENDING
-> task_event 写入 TASK_CREATED
-> task_outbox 写入 TASK_DISPATCH_REQUESTED
-> 数据库事务提交
-> Outbox Dispatcher 扫描并 claim outbox
-> 发送 RabbitMQ
-> Consumer 接收 TaskDispatchMessage
-> Atomic Claim：PENDING / RETRY_PENDING -> RUNNING
-> 创建 task_attempt
-> Prompt Template + ModelRouter + MockLlmClient
-> 保存 attempt metadata / output chunks
-> task -> SUCCESS / RETRY_PENDING / FAILED / CANCELLED
```

要点：

1. `createTask` 不直接发送 RabbitMQ，由 Outbox Dispatcher 负责投递。
2. Consumer 通过 atomic claim 防止重复执行。
3. `task_attempt` 记录每次执行尝试，不被 `task` 最终摘要覆盖。

---

## 5. 核心链路二：RAG Retrieval / VectorStore

```text
POST /documents（上传）
-> DocumentChunker 切分
-> POST /documents/{documentId}/embeddings
-> EmbeddingProvider 生成向量
-> VectorStore.upsert（默认 ExactCosineVectorStore 写入 document_chunk_embedding）
-> POST /documents/search
-> VectorStore.search（exact cosine 或 Qdrant）
-> POST /evaluations/retrieval（检索指标评估）
-> POST /rag/answer（检索 + Mock LLM 回答 + citation）
```

默认配置：

```properties
app.embedding.provider=mock
app.vector-store.provider=exact
```

启用 Qdrant（手工验证，非默认测试）：

```properties
app.vector-store.provider=qdrant
app.vector-store.qdrant.base-url=http://127.0.0.1:6333
app.vector-store.qdrant.initialize-collection=true
```

---

## 6. 本地快速启动

详细说明见 [docs/local-dev.md](docs/local-dev.md)。

```powershell
cd E:\code\ai-task-orchestrator
docker compose up -d
.\mvnw.cmd spring-boot:run "-Dspring-boot.run.profiles=docker"
```

PowerShell 不要使用 `cd /d E:\code\ai-task-orchestrator`。

---

## 7. 可运行 Demo 入口

**Web UI（V3.1 产品入口）**

| 页面 | 路径 | 说明 |
| --- | --- | --- |
| 产品首页 | `/` 或 `/index.html` | 能力说明与导航 |
| 知识库问答 | `/ask.html`（兼容 `/rag-demo.html`） | 提问、Citations、Metadata |
| 文档浏览 / 上传 | `/documents.html` · `POST /documents/upload`（txt/md/pdf 同步 ingestion） |
| 评估说明 | `/evaluation.html` | 指标与报告路径 |

完整 E2E 演示见：[docs/demo/e2e-demo.md](docs/demo/e2e-demo.md)（配合 `docs/demo/task-flow.http` 与 `docs/demo/rag-flow.http`）。

| 能力 | HTTP 入口 |
| --- | --- |
| 文档上传（同步 ingestion） | `POST /documents/upload` |
| 文档列表（只读） | `GET /documents` |
| 创建任务 | `POST /tasks` |
| 查询任务 | `GET /tasks/{taskId}` |
| 查询 attempts | `GET /tasks/{taskId}/attempts` |
| 查询 output chunks | `GET /tasks/{taskId}/output-chunks` |
| 上传文档 | `POST /documents` |
| 生成 embedding | `POST /documents/{documentId}/embeddings` |
| 文档检索 | `POST /documents/search` |
| 检索评估 | `POST /evaluations/retrieval` |
| RAG 问答 | `POST /rag/answers` |

开发/调试接口（非生产）：

| 能力 | HTTP 入口 |
| --- | --- |
| 重复投递测试 | `POST /dev/tasks/{taskId}/dispatch` |
| 修改任务状态 | `PATCH /dev/tasks/{taskId}/status`（需 `dev` profile） |

更多请求示例见 [docs/api-examples.md](docs/api-examples.md)。

---

## 8. 测试与评估

**默认单元/集成测试**

```powershell
.\mvnw.cmd test
```

默认测试要求：

- 不启动 Docker（CI 除外）
- 不连接 Qdrant
- 不访问 OpenAI API
- 不启动 Python embedding worker
- 不下载 embedding 模型
- 不访问外部网络

测试数量以本地或 CI 实际输出为准，不要手写猜测数字。

**Benchmark / Comparison（测试 harness，非生产 API）**

| Harness | 说明 |
| --- | --- |
| `BenchmarkRunnerEvidenceMapperTest` | benchmark seed + evidence 映射 + evaluation |
| `EmbeddingProviderBenchmarkComparisonTest` | baseline vs candidate embedding provider |
| `VectorStoreBenchmarkComparisonTest` | ExactCosineVectorStore vs fake candidate |
| `ChunkingStrategyComparisonTest` | Fixed vs Adaptive chunking |

这些能力通过测试与 benchmark runner 验证，尚未暴露为独立 HTTP benchmark API。

---

## 9. 当前限制

- LLM 默认仍为 Mock，不代表真实生成质量。
- Embedding 默认仍为 Mock，不代表真实语义效果。
- RAG Answer 基于 Mock LLM，未做 Generation Evaluation。
- `ExactCosineVectorStore` 为 exact scan，不适合大规模生产检索。
- Qdrant 接入为实验性实现，无 Docker Compose 集成与生产级运维。
- Local Embedding Worker 需手工启动 Python 环境。
- 无多租户、鉴权、限流、完整 observability。
- Web UI 为 MVP：无登录、无权限、无文档删除/编辑；支持 Documents 页面上传 txt/md/pdf。
- 不是完整 Agent Runtime。

---

## 10. Roadmap

从当前真实状态继续：

1. **E2E Demo Golden Path** — 端到端演示脚本与验收路径
2. **Atomic Finalization** — 任务终态与 outbox 边界进一步硬化
3. **Qdrant Manual Verification** — 手工 external Qdrant benchmark 与运行手册
4. **API Error Response Standardization** — 统一错误响应格式
5. **Local Embedding Worker Packaging** — Docker 化与依赖治理
6. **Retrieval Policy & VIP Search** — 按用户计划调整 topK / 检索策略
7. **RAG Answer Hardening** — 生产级 citation / generation 质量治理
8. **Generation Evaluation** — Faithfulness、Answer Relevance、LLM-as-a-judge
9. **Agent Runtime**
10. **KV Cache-aware Scheduling**
11. **Product UI** — 登录、权限、文档上传与管理

已完成能力（V2.4–V3.1）不再列入待做：Retrieval Evaluation、EmbeddingProvider、Local Worker 接入、VectorStore 抽象、QdrantVectorStore、VectorStore Benchmark Harness、Rerank、Hybrid Fusion、产品化 Web 入口。

---

## 11. 面试文档索引

分版本 deep-dive 见 [docs/interview](docs/interview)，例如：

| 版本 | 主题 |
| --- | --- |
| V0.x | Task / Outbox / Retry / RabbitMQ |
| V1.x | LLM / Prompt / Model Router / Output Chunks |
| V2.1 | Document Upload & Chunking |
| V2.2 | Mock Embedding & Vector Search |
| V2.2.x | Production Hardening |
| V2.3 | RAG Answer with Citation |
| V2.4.x | Retrieval Evaluation / Metrics / Dataset / Benchmark |
| V2.5 | Real Embedding Provider |
| V2.5.1 | Embedding Provider Benchmark Comparison |
| V2.5.2 | Local Embedding Worker |
| V2.6 | VectorStore Abstraction |
| V2.6.1 | Qdrant Integration |
| V2.6.2 | VectorStore Benchmark |

简历与面试表达见 [docs/resume-interview.md](docs/resume-interview.md)。

---

## 相关文档

- [docs/manual/knowledge-base-lifecycle-management.md](docs/manual/knowledge-base-lifecycle-management.md)
- [docs/local-dev.md](docs/local-dev.md)
- [docs/api-examples.md](docs/api-examples.md)
- [docs/project-structure.md](docs/project-structure.md)
- [docs/resume-interview.md](docs/resume-interview.md)
