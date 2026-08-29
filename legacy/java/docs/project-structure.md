# 项目结构说明

## 一、文档目的

本文档帮助开发者理解 AI Task Orchestrator 的模块划分、核心类职责、任务执行链路、RAG / Embedding / VectorStore 链路，以及当前架构边界。

---

## 二、整体目录结构

```text
src/main/java/com/tuoman/ai_task_orchestrator
├── config              # RabbitMQ 等基础设施配置
├── controller          # HTTP API 入口
├── document            # 文档 chunking 策略
├── dto                 # 请求 / 响应对象
├── embedding           # EmbeddingProvider 抽象与实现
├── entity              # JPA 实体
├── enums               # 状态与类型枚举
├── evaluation          # Benchmark / Evidence Mapper（测试 harness 为主）
├── llm                 # LLM Client / ModelRouter
├── mq                  # RabbitMQ 生产 / 消费
├── prompt              # Prompt Template 渲染
├── repository          # 数据访问
├── scheduler           # 重试 / 超时 / Outbox 调度
├── service             # 核心业务逻辑
├── state               # 任务状态机
├── vectorstore         # VectorStore 抽象与实现
│   └── qdrant          # Qdrant REST 客户端与 DTO
└── AiTaskOrchestratorApplication.java

src/main/resources
├── application.properties
├── application-docker.properties
└── db/migration        # Flyway 迁移脚本

workers/embedding-worker   # Python FastAPI 本地 embedding worker（实验性）
docs/                      # 项目文档
docs/interview/            # 版本演进面试文档（只读索引，本版本不修改）
```

---

## 三、controller 包

Controller 层负责接收 HTTP 请求，不包含业务逻辑。

| Controller | 路径 | 职责 |
| --- | --- | --- |
| `TaskController` | `/tasks` | 创建 / 查询 / 取消任务；查询 output chunks 与 attempts |
| `DevTaskDispatchController` | `/dev/tasks` | 本地重复投递 MQ（非生产） |
| `DevTaskController` | `/dev/tasks` | dev profile 下修改任务状态（调试） |
| `DocumentController` | `/documents` | 上传、查询、chunk、embedding、search |
| `RetrievalEvaluationController` | `/evaluations` | 检索评估 API |
| `RagAnswerController` | `/rag` | RAG 问答 API（Mock LLM） |

真实 API 路径：

- `POST /tasks`、`GET /tasks/{taskId}`、`POST /tasks/{taskId}/cancel`
- `GET /tasks/{taskId}/output-chunks`、`GET /tasks/{taskId}/attempts`
- `POST /dev/tasks/{taskId}/dispatch`
- `PATCH /dev/tasks/{taskId}/status`（需 `dev` profile）
- `POST /documents`、`GET /documents/{documentId}`、`GET /documents/{documentId}/chunks`
- `POST /documents/{documentId}/embeddings`、`POST /documents/search`
- `POST /evaluations/retrieval`
- `POST /rag/answer`

---

## 四、task 模块

**职责**：任务生命周期管理、状态流转、事件追踪。

核心类：

- `TaskController` / `TaskService`：创建任务、查询详情、取消、状态变更
- `TaskEntity` / `TaskRepository`：任务持久化
- `TaskEventEntity` / `TaskEventRepository`：历史事件（`TASK_CREATED`、`STATUS_CHANGED`）
- `TaskStateMachine`：限制合法状态流转（`PENDING` → `RUNNING` → `SUCCESS` 等）
- `TaskExecutionService`：Consumer 执行入口，调用 LLM、保存结果
- `TaskOutputChunkService`：将 LLM 输出拆分为 chunks 并持久化

执行链路：

```text
POST /tasks
-> TaskService 创建 PENDING 任务 + 写 task_event
-> TaskOutboxService 写入 outbox
-> Outbox Dispatcher 投递 RabbitMQ
-> Consumer -> Atomic Claim -> TaskExecutionService
-> ModelRouter / PromptTemplate / LlmClient
-> 更新 task_attempt、task_output_chunk、task 状态
```

---

## 五、outbox 模块

**职责**：Transactional Outbox，保证任务创建与 MQ 投递的一致性。

核心类：

- `TaskOutboxEntity` / `TaskOutboxRepository`：outbox 记录
- `TaskOutboxService`：创建任务时同事务写入 outbox
- `TaskOutboxDispatcherScheduler`：扫描 pending outbox 并投递 MQ

与直接 `Producer.send` 的区别：outbox 在同一数据库事务内写入，Scheduler 异步投递，避免“DB 已提交但 MQ 未发送”的不一致。

---

## 六、attempt 模块

**职责**：记录每次任务执行尝试（provider、model、status、token、latency）。

核心类：

- `TaskAttemptEntity` / `TaskAttemptRepository`
- `TaskAttemptService`：创建 attempt、更新执行结果
- `GET /tasks/{taskId}/attempts`：查询历史尝试

与 task 表的关系：`task` 保存当前聚合状态；`task_attempt` 保存每次执行明细，便于重试审计与 LLM 调用追踪。

---

## 七、llm / prompt / model router

**llm 包**

- `LlmClient`：统一 LLM 调用接口
- `MockLlmClient`：当前默认实现，不调用外部 API
- `LlmRequest` / `LlmResponse`：请求与响应对象
- `ModelRouter`：根据 `requestedModel` 选择实际执行模型（`mock-llm` / `mock-fast` / `mock-smart`）

**prompt 包**

- `PromptTemplateRenderer`：渲染 `{{prompt}}`、`{{taskId}}`、`{{model}}`
- `PromptTemplateEntity`：数据库模板定义，默认 `default_task_prompt`

TaskExecutionService 在每次执行时渲染 prompt、调用 LlmClient、保存 LLM usage metadata 到 task 与 task_attempt。

---

## 八、output chunk 模块

**职责**：持久化 LLM 输出片段，供后续 polling 或 streaming 扩展。

- `TaskOutputChunkEntity` / `TaskOutputChunkRepository`
- `TaskOutputChunkService`：拆分并保存 chunks
- `GET /tasks/{taskId}/output-chunks`

当前不是 SSE / WebSocket，只是异步任务下的持久化片段查询。

---

## 九、document / chunking 模块

**职责**：文档上传、文本读取、chunk 切分与 metadata 保存。

**document 包**

- `DocumentChunker`：Fixed / Adaptive 两种切分策略
- `DocumentChunkResult`：切分结果对象

**service 层**

- `DocumentService`：校验 `.txt` / `.md`、UTF-8 读取、调用 chunker、保存 `document` / `document_chunk`

Chunk metadata 包括：`chunkIndex`、`content`、`headingPath`、`chunkStrategy`、`startOffset`、`endOffset`。

Adaptive Chunking 按 Markdown 标题结构切分；Fixed Chunking 按固定长度切分。对比通过 `ChunkingStrategyComparisonTest` 验证，无独立 HTTP API。

---

## 十、embedding 模块

**职责**：文本向量化抽象，支持多种 provider 切换。

**embedding 包**

| 类 | 职责 |
| --- | --- |
| `EmbeddingProvider` | 统一 embedding 接口 |
| `MockEmbeddingProvider` | 默认 provider，确定性 mock 向量 |
| `OpenAiCompatibleEmbeddingProvider` | OpenAI-compatible HTTP embedding |
| `LocalEmbeddingWorkerEmbeddingProvider` | 调用 Python worker |
| `EmbeddingProviderConfiguration` | 按 `app.embedding.provider` 装配 bean |
| `RestClientOpenAiEmbeddingHttpClient` | OpenAI HTTP 客户端 |
| `RestClientLocalEmbeddingWorkerClient` | Local worker HTTP 客户端 |

**service 层**

- `DocumentEmbeddingService`：为文档所有 chunk 生成 embedding，写入 `document_chunk_embedding` 与 VectorStore

配置项：

```properties
app.embedding.provider=mock|openai|local-worker
app.embedding.local-worker.base-url=http://127.0.0.1:8001
```

---

## 十一、vectorstore 模块

**职责**：向量存储与检索抽象，与 embedding 持久化解耦。

**vectorstore 包**

| 类 | 职责 |
| --- | --- |
| `VectorStore` | upsert / search / delete 接口 |
| `ExactCosineVectorStore` | 默认 baseline，基于 DB embedding 做 exact cosine |
| `VectorStoreConfiguration` | 按 `app.vector-store.provider` 装配 |
| `VectorSearchRequest` / `VectorSearchResult` | 检索请求与结果 |
| `VectorStoreProperties` | 配置属性 |

**vectorstore.qdrant 包**（实验性）

| 类 | 职责 |
| --- | --- |
| `QdrantVectorStore` | Qdrant 实现 |
| `RestClientQdrantVectorStoreClient` | Qdrant REST API 客户端 |
| `QdrantPayloadMapper` | chunk metadata ↔ Qdrant payload 映射 |
| DTO 类 | search / upsert / filter 请求响应 |

`DocumentEmbeddingService.search()` 委托给 active `VectorStore`，不再内联 cosine 计算。

配置项：

```properties
app.vector-store.provider=exact|qdrant
app.vector-store.qdrant.base-url=http://127.0.0.1:6333
app.vector-store.qdrant.initialize-collection=false
```

---

## 十二、retrieval evaluation 模块

**职责**：对检索质量做离线评估，输出 Recall@K、Precision@K、MRR、NDCG@K 等指标。

**service 层**

- `RetrievalEvaluationService`：执行评估逻辑
- `RetrievalMetricsCalculator`：计算各 @K 指标

**controller**

- `POST /evaluations/retrieval`

**evaluation 包**（benchmark harness）

- `BenchmarkEvidenceMapper`：将 benchmark seed 中的 `expectedEvidenceIds` 映射为真实 `expectedChunkIds`
- `RetrievalBenchmarkDataset` / `RetrievalBenchmarkCase`：benchmark 数据集结构
- `RetrievalBenchmarkResourceLoader`：加载 `src/test/resources/evaluation/` 资源

Benchmark seed 资源：

- `retrieval-corpus-v1.md`
- `retrieval-benchmark-v1.json`

Evidence Mapper 与 seed 加载主要用于测试 harness，未单独暴露 HTTP API。

---

## 十三、benchmark 模块

**职责**：对比不同 provider / vectorstore / chunking 策略的效果与延迟。

**evaluation 包**

| 类 | 职责 |
| --- | --- |
| `VectorStoreBenchmarkRunner` | 对比 baseline vs candidate VectorStore |
| `LatencyMeasuringVectorStore` | 包装 VectorStore 测量延迟 |
| `VectorStoreMetricDelta` | 指标差异记录 |
| `BenchmarkEvidenceMapper` | evidence → chunkId 映射 |

测试入口（无生产 HTTP API）：

- `VectorStoreBenchmarkComparisonTest`：exact vs fake candidate
- `EmbeddingProviderBenchmarkComparisonTest`：embedding provider 对比
- `BenchmarkRunnerEvidenceMapperTest`：evidence mapper 验证
- `ChunkingStrategyComparisonTest`：fixed vs adaptive chunking

---

## 十四、RAG answer 模块

**职责**：检索 + Mock LLM 生成带 citation 的回答。

- `RagAnswerController`：`POST /rag/answer`
- `RagAnswerService`：调用 `DocumentEmbeddingService.search()`，再调用 `LlmClient`
- `RagPromptBuilder`：构造 RAG prompt

链路已打通，但 LLM 仍为 Mock，属于基础原型，不是 production-grade generation。

---

## 十五、mq / scheduler 模块

**mq 包**

- `RabbitMQConfig`：exchange / queue / binding
- `TaskDispatchProducer` / `TaskDispatchConsumer`：消息投递与消费

**scheduler 包**

- `TaskRetryScheduler`：扫描 `RETRY_PENDING` 到期任务，重新投递
- `TaskTimeoutScheduler`：扫描 `RUNNING` 超时任务，标记 `FAILED`
- `TaskOutboxDispatcherScheduler`：扫描 outbox 并投递 MQ

---

## 十六、workers 目录

**职责**：Python FastAPI 本地 embedding worker（实验性，非默认测试依赖）。

```text
workers/embedding-worker/
├── main.py              # FastAPI app，POST /embeddings
└── requirements.txt     # sentence-transformers 等依赖
```

Java 侧通过 `LocalEmbeddingWorkerEmbeddingProvider` + `RestClientLocalEmbeddingWorkerClient` 调用。需手工启动 worker 并配置 `app.embedding.provider=local-worker`。

---

## 十七、docs/interview 目录

**职责**：按版本记录功能演进、设计决策与面试表达，供 README 索引引用。

示例文档（本版本不修改）：

- V0.x：任务状态机、MQ、重试、幂等、取消超时
- V1.x：LLM 抽象、Prompt Template、Model Router、output chunks
- V2.1–V2.4：Document、Chunking、Retrieval Evaluation
- V2.5–V2.6：Embedding Provider、Local Worker、VectorStore、Qdrant、Benchmark

---

## 十八、数据库迁移（Flyway）

| 版本 | 内容 |
| --- | --- |
| V1–V11 | task、task_event、retry/timeout/LLM 字段、prompt_template、task_output_chunk |
| V12 | document / document_chunk |
| V13 | chunk metadata（headingPath、chunkStrategy 等） |
| V14 | document_chunk_embedding |
| V15 | task_outbox |
| V16 | task_attempt |

---

## 十九、核心链路一：Reliable Async Task Execution

```text
1. POST /tasks
2. TaskService 创建 task（PENDING）+ task_event + task_outbox（同事务）
3. TaskOutboxDispatcherScheduler 扫描 outbox -> RabbitMQ
4. TaskDispatchConsumer 收到消息
5. Atomic Claim：PENDING/RETRY_PENDING -> RUNNING
6. TaskAttemptService 创建 attempt
7. ModelRouter -> PromptTemplateRenderer -> MockLlmClient
8. 保存 LLM metadata、output chunks
9. SUCCESS / RETRY_PENDING / FAILED / CANCELLED
10. RetryScheduler / TimeoutScheduler 后台扫描
```

---

## 二十、核心链路二：RAG Retrieval / VectorStore

```text
1. POST /documents（上传 .txt/.md）
2. DocumentService -> DocumentChunker（fixed/adaptive）-> document_chunk
3. POST /documents/{documentId}/embeddings
4. EmbeddingProvider.embed -> document_chunk_embedding + VectorStore.upsert
5. POST /documents/search
6. EmbeddingProvider.embed(query) -> VectorStore.search -> TopK chunks
7. POST /evaluations/retrieval（可选，传入 cases + expectedChunkIds）
8. POST /rag/answer（可选，检索 + Mock LLM + citations）
```

---

## 二十一、当前架构边界

**已实现**

- 可靠异步任务编排（outbox、atomic claim、attempt、retry、timeout）
- Mock LLM / Prompt Template / Model Router
- Document Upload / Fixed & Adaptive Chunking
- EmbeddingProvider 抽象（mock / openai / local-worker）
- VectorStore 抽象（exact / qdrant）
- Document search、Retrieval Evaluation、RAG Answer（Mock LLM）
- Benchmark harness（测试验证）

**原型 / 实验性**

- Local Embedding Worker、QdrantVectorStore、OpenAI embedding 手工配置
- RAG Answer（Mock LLM，未做 Generation Evaluation）

**尚未实现**

- Rerank、Hybrid Search
- Production-grade RAG generation
- Auth / tenant / quota、API rate limit
- Agent Runtime、KV Cache-aware scheduling
- Production observability dashboard
- Distributed worker registry

---

## 二十二、相关文档

- [README.md](../README.md)
- [docs/local-dev.md](local-dev.md)
- [docs/api-examples.md](api-examples.md)
- [docs/resume-interview.md](resume-interview.md)
