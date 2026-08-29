# AI Task Orchestrator E2E Demo

本指南提供一条 **5 分钟内可跑通** 的端到端演示路径，帮助 reviewer快速理解项目两条核心链路：

1. **异步任务执行**（Outbox → RabbitMQ → Atomic Claim → Task Attempt → Output Chunks）
2. **RAG 检索**（Document Upload → Chunking → Embedding → Vector Search → Retrieval Evaluation）

配套 HTTP 请求文件：

- 任务链路：`docs/demo/task-flow.http`
- RAG 链路：`docs/demo/rag-flow.http`

---

## 1. Demo 目标

| Demo | 说明 | HTTP 文件 |
| --- | --- | --- |
| **Demo A** | 创建任务 → 异步执行 → 查询状态 / attempts / output chunks | `task-flow.http` |
| **Demo B** | 上传 `.md` 文档 → 查看 chunks | `rag-flow.http` |
| **Demo C** | 生成 embedding → 语义检索 TopK | `rag-flow.http` |
| **Demo D** | Retrieval Evaluation（检索指标，非生成质量） | `rag-flow.http` |
| **Demo E** | Qdrant / Local Worker 可选手工验证 | 本文第 9 节 |

**默认 demo 约束（重要）**

- 使用 **mock embedding** + **exact cosine VectorStore** baseline
- **不依赖** Qdrant
- **不依赖** OpenAI API
- **不依赖** Python embedding worker
- **不下载** embedding 模型
- **不访问** 外部网络（除本机 Docker MySQL / RabbitMQ）

Qdrant 与 local-worker 仅在 Demo E 作为**可选手工验证**说明，不是默认 demo 路径。

---

## 2. 前置条件

- JDK 21+
- Docker Desktop（MySQL + RabbitMQ）
- PowerShell
- IDE 支持 `.http` 文件（IntelliJ IDEA / VS Code REST Client 插件）

进入项目目录：

```powershell
cd E:\code\ai-task-orchestrator
```

不要使用：

```powershell
cd /d E:\code\ai-task-orchestrator
```

确认默认配置（`application.properties` / `application-docker.properties`）：

```properties
app.embedding.provider=mock
app.vector-store.provider=exact
```

---

## 3. 启动基础设施

```powershell
cd E:\code\ai-task-orchestrator
docker compose up -d
docker compose ps
```

等待 MySQL、RabbitMQ 容器 healthy。

| 服务 | 本机端口 |
| --- | --- |
| MySQL | `3307` |
| RabbitMQ AMQP | `5672` |
| RabbitMQ 管理台 | `15672`（guest / guest） |

---

## 4. 启动 Spring Boot

新开一个 PowerShell 窗口：

```powershell
cd E:\code\ai-task-orchestrator
.\mvnw.cmd spring-boot:run "-Dspring-boot.run.profiles=docker"
```

看到应用监听 `8080` 后，再执行 `.http` 请求。

更多细节见 [docs/local-dev.md](../local-dev.md)。

---

## 5. Demo A：异步任务执行链路

**目标**：演示任务创建、MQ 异步调度、执行完成后的状态与输出查询。

**步骤**（按 `docs/demo/task-flow.http` 顺序执行）：

1. `POST /tasks` — 创建任务（`prompt` 必填，可选 `model`）
2. 等待数秒（Consumer 异步执行 Mock LLM）
3. `GET /tasks/{taskId}` — 确认 `status = SUCCESS`
4. `GET /tasks/{taskId}/attempts` — 查看执行尝试（provider、model、token、latency）
5. `GET /tasks/{taskId}/output-chunks` — 查看持久化输出片段

**观察要点**

- 创建时 `status = PENDING`，执行后变为 `SUCCESS`
- `task_attempt` 记录每次 LLM 调用明细
- `output-chunks` 将完整回答拆分为可查询片段

**可选**：`prompt` 含 `fail` 或 `失败` 可触发 Mock 失败与重试（见 `task-flow.http` 注释）。

---

## 6. Demo B：文档上传与 Chunking

**目标**：演示文档上传、自动 chunking 与 chunks 查询。

**推荐测试文件**：`src/test/resources/evaluation/retrieval-corpus-v1.md`（项目自带，含 Outbox / Atomic Claim / Retrieval Evaluation 等段落）

**步骤**（`rag-flow.http` 前半部分）：

1. `POST /documents` — multipart 上传 `file`（仅 `.txt` / `.md`）
2. 记录响应中的 `documentId`、`chunkCount`
3. `GET /documents/{documentId}` — 文档元信息
4. `GET /documents/{documentId}/chunks` — 查看 chunk 列表（含 `chunkId`、`content`、`headingPath`、`chunkStrategy` 等）

**观察要点**

- 上传成功后 `status = CHUNKED`，`chunkCount > 0`
- Adaptive Chunking 会按 Markdown 标题结构切分

---

## 7. Demo C：Embedding 与 Vector Search

**目标**：在 mock embedding + exact VectorStore 下完成向量写入与 TopK 检索。

**步骤**（`rag-flow.http` 中段）：

1. `POST /documents/{documentId}/embeddings` — 为所有 chunk 生成向量
2. 响应含 `embeddingProvider`、`embeddingModel`、`dimension`、`distanceMetric`
3. `POST /documents/search` — 传入 `query`、`topK`，可选 `documentId` 限定单文档

**示例 query**（与测试语料匹配）：

```text
为什么 createTask 不直接发送 RabbitMQ
```

**观察要点**

- 默认 `embeddingProvider = mock`，`distanceMetric` 为 cosine 相关度量
- search 结果含 `chunkId`、`score`、`content`
- 检索与 embedding 必须使用同一 provider/model 空间

---

## 8. Demo D：Retrieval Evaluation

**目标**：用标注 cases 量化检索质量（**只评价检索，不评价生成答案事实性**）。

**步骤**（`rag-flow.http` 后段）：

1. 确保已完成 Demo B + Demo C（文档已 embed）
2. 从 `GET /documents/{documentId}/chunks` 获取真实 `chunkId`
3. `POST /evaluations/retrieval` — 传入 `documentId`、`topKValues`、`cases`（含 `expectedChunkIds`）

**请求字段**（与 `RetrievalEvaluationRequest` 一致）：

- `documentId`：Long
- `topKValues`：如 `[1, 3, 5]`
- `cases[].caseId`、`cases[].query`、`cases[].expectedChunkIds`（真实 chunk 主键）

**响应指标**：Recall@K、Precision@K、HitRate@K、MRR、NDCG@K、ContextPrecision@K 等。

**说明**

- Retrieval Evaluation **只衡量检索是否命中预期 chunk**，不验证 RAG 生成答案是否正确
- Benchmark seed（`retrieval-benchmark-v1.json`）与 Evidence Mapper 主要用于**测试 harness**，未暴露为独立 HTTP API
- 手工 demo 需自行根据 chunks 内容填写 `expectedChunkIds`

**可选扩展**（非 rag-flow.http 必做）：`POST /rag/answer` 可演示检索 + Mock LLM + citation，但 LLM 仍为 Mock，不代表生产生成质量。

---

## 9. Demo E：Qdrant / Local Worker 可选手工验证

以下**不是默认 demo**，需额外启动外部服务并修改配置。默认 `mvn test` 与默认 demo 均不依赖它们。

### Local Embedding Worker（实验性）

```powershell
cd E:\code\ai-task-orchestrator\workers\embedding-worker
python -m venv .venv
.\.venv\Scripts\Activate.ps1
pip install -r requirements.txt
uvicorn main:app --host 127.0.0.1 --port 8001
```

Java 配置：

```properties
app.embedding.provider=local-worker
app.embedding.local-worker.base-url=http://127.0.0.1:8001
```

切换 provider 后需**重新** `POST /documents/{documentId}/embeddings`。

### Qdrant（实验性）

```powershell
docker run -p 6333:6333 -p 6334:6334 qdrant/qdrant
```

Java 配置：

```properties
app.vector-store.provider=qdrant
app.vector-store.qdrant.base-url=http://127.0.0.1:6333
app.vector-store.qdrant.initialize-collection=true
```

Qdrant **未纳入**项目 `docker-compose.yml`，属于手工验证路径，**不要声称**已完成 production Qdrant benchmark。

---

## 10. 常见问题

### 任务一直 PENDING

- 确认 RabbitMQ 容器运行中
- 确认 Spring Boot 使用 `docker` profile
- 查看应用日志是否有 Consumer 错误

### 文档上传失败

- 仅支持 `.txt` / `.md`
- 文件需为有效 UTF-8 文本

### Search 无结果

- 先执行 `POST /documents/{documentId}/embeddings`
- `documentId` 过滤是否与 embed 的文档一致
- provider/model 是否与 embed 时一致（默认均为 mock）

### Retrieval Evaluation 指标全 0

- `expectedChunkIds` 必须是该文档的真实 chunk 主键
- query 应与语料内容相关
- 确认已完成 embedding

### `.http` 变量不生效

- 创建任务 / 上传文档后，将响应中的 `taskId` / `documentId` 填入文件顶部变量
- IntelliJ 可使用响应脚本自动保存（见 `task-flow.http` 注释）

---

## 11. 本 demo 不覆盖什么

以下能力**未纳入默认 E2E demo**，或未暴露为 HTTP API：

| 能力 | 说明 |
| --- | --- |
| VectorStore Benchmark | 仅 `VectorStoreBenchmarkComparisonTest` 等测试 harness |
| Embedding Provider Benchmark | 仅测试 harness |
| Fixed vs Adaptive Chunking 对比 | 仅 `ChunkingStrategyComparisonTest` |
| Evidence Mapper + benchmark seed 自动化 | 测试资源 + harness，无独立 HTTP API |
| Rerank / Hybrid Search | 尚未实现 |
| Agent Runtime | 尚未实现 |
| Auth / tenant / quota | 尚未实现 |
| Production observability | 尚未实现 |
| Qdrant / Local Worker 默认路径 | 仅 Demo E 可选手工验证 |

---

## 相关文档

- [docs/local-dev.md](../local-dev.md)
- [docs/api-examples.md](../api-examples.md)
- [docs/project-structure.md](../project-structure.md)
- [README.md](../../README.md)
