# API 验收文档

本文档记录 AI Task Orchestrator **当前代码中真实存在** 的 HTTP API 调用示例与验收步骤。路径与字段以当前 Controller 为准。

## 一、前置条件

- Docker Compose 已启动 MySQL 和 RabbitMQ
- Spring Boot 已用 `docker` profile 启动
- Flyway 迁移已执行成功

本地启动见 [docs/local-dev.md](local-dev.md)。

默认配置：

- `app.embedding.provider=mock`
- `app.vector-store.provider=exact`

---

## 二、任务 API

### 2.1 创建任务

```http
POST http://localhost:8080/tasks
Content-Type: application/json
```

```json
{
  "prompt": "normal task"
}
```

指定模型：

```json
{
  "prompt": "normal task",
  "model": "mock-fast"
}
```

期望：`status = PENDING`，返回 `taskId`。

### 2.2 查询任务

```http
GET http://localhost:8080/tasks/{taskId}
```

主要字段：`status`、`requestedModel`、`llmModel`、`llmProvider`、token usage、`resultContent` 等。

### 2.3 取消任务

```http
POST http://localhost:8080/tasks/{taskId}/cancel
```

### 2.4 查询 task attempts

```http
GET http://localhost:8080/tasks/{taskId}/attempts
```

返回该任务的执行尝试列表（provider、model、status、token、latency 等）。

### 2.5 查询 output chunks

```http
GET http://localhost:8080/tasks/{taskId}/output-chunks
```

---

## 三、开发 / 调试 API

### 3.1 重复投递（非生产）

```http
POST http://localhost:8080/dev/tasks/{taskId}/dispatch
```

### 3.2 修改任务状态（dev profile）

```http
PATCH http://localhost:8080/dev/tasks/{taskId}/status
Content-Type: application/json
```

```json
{
  "status": "CANCELLED",
  "message": "手动取消任务"
}
```

需启用 Spring profile `dev`。该接口用于状态机调试，不是正式用户 API。

---

## 四、文档 API

### 4.1 上传文档

```http
POST http://localhost:8080/documents
Content-Type: multipart/form-data
```

参数：`file`（`.txt` / `.md`）

期望：`status = CHUNKED`，`chunkCount > 0`。

### 4.2 查询文档

```http
GET http://localhost:8080/documents/{documentId}
```

### 4.3 查询文档 chunks

```http
GET http://localhost:8080/documents/{documentId}/chunks
```

响应含 `chunkIndex`、`content`、`headingPath`、`chunkStrategy`、`startOffset`、`endOffset` 等。

---

## 五、Embedding 与检索 API

### 5.1 生成文档 embedding

使用当前 active `EmbeddingProvider`（默认 mock）为文档所有 chunk 生成向量并写入 VectorStore。

```http
POST http://localhost:8080/documents/{documentId}/embeddings
```

响应含 `embeddingProvider`、`embeddingModel`、`dimension`、`distanceMetric`、写入条数等。

### 5.2 文档语义检索

```http
POST http://localhost:8080/documents/search
Content-Type: application/json
```

```json
{
  "query": "为什么使用 transactional outbox",
  "topK": 5,
  "documentId": 1
}
```

可选字段：

- `embeddingProvider` / `embeddingModel`：限定 embedding 空间；省略时使用当前 active provider
- `documentId`：限定单文档检索；省略时为全局（同 provider/model 空间内）

响应含 `chunkId`、`score`、`content`、`embeddingProvider`、`embeddingModel`、`distanceMetric` 等。

手工切换 OpenAI-compatible embedding（需 API key，非默认测试）：

```properties
app.embedding.provider=openai
app.embedding.openai.api-key=${OPENAI_API_KEY}
```

手工切换 Qdrant VectorStore（需本地 Qdrant，非默认测试）：

```properties
app.vector-store.provider=qdrant
app.vector-store.qdrant.base-url=http://127.0.0.1:6333
app.vector-store.qdrant.initialize-collection=true
```

---

## 六、Retrieval Evaluation API

```http
POST http://localhost:8080/evaluations/retrieval
Content-Type: application/json
```

```json
{
  "documentId": 1,
  "topKValues": [1, 3, 5],
  "cases": [
    {
      "caseId": "outbox-001",
      "query": "为什么 createTask 不应该直接发送 RabbitMQ？",
      "expectedChunkIds": [12, 13]
    }
  ]
}
```

说明：

- 需先对文档执行 embedding（`POST /documents/{documentId}/embeddings`）
- `expectedChunkIds` 为真实 chunk 主键，不是 evidence marker 字符串
- 响应含 per-case metrics 与 summary（Recall@K、Precision@K、MRR、NDCG@K 等）

Benchmark seed 资源位于 `src/test/resources/evaluation/`，通过测试 harness 将 `expectedEvidenceIds` 映射为 `expectedChunkIds`。该映射逻辑未单独暴露为 HTTP API。

---

## 七、RAG Answer API

```http
POST http://localhost:8080/rag/answer
Content-Type: application/json
```

```json
{
  "query": "Transactional Outbox 解决什么问题？",
  "documentId": 1,
  "topK": 5,
  "requestedModel": "mock-llm"
}
```

响应含：

- `answer`：当前为 Mock LLM 生成
- `citations`：检索 chunk 引用
- `retrieval`：检索 metadata（provider、model、topK 等）
- `llm`：LLM metadata

说明：链路已打通，但 LLM 仍为 Mock，不代表生产级生成质量。

---

## 八、Benchmark / Comparison（无生产 HTTP API）

以下能力**仅通过测试 / benchmark runner 验证**，未暴露为 HTTP API：

| 能力 | 测试入口 |
| --- | --- |
| Evidence Mapper + benchmark seed | `BenchmarkRunnerEvidenceMapperTest` |
| Embedding Provider 对比 | `EmbeddingProviderBenchmarkComparisonTest` |
| VectorStore 对比（exact vs fake candidate） | `VectorStoreBenchmarkComparisonTest` |
| Fixed vs Adaptive Chunking 对比 | `ChunkingStrategyComparisonTest` |

如需手工 external Qdrant benchmark，需本地启动 Qdrant 并切换 `app.vector-store.provider=qdrant`，属于手工验证流程，不是默认 `mvn test`。

---

## 九、任务链路验收示例

**正常任务**

1. `POST /tasks`，`prompt = normal task`
2. 等待 Consumer 执行
3. `GET /tasks/{taskId}` → `SUCCESS`
4. `GET /tasks/{taskId}/output-chunks` 有数据

**失败重试**

```json
{ "prompt": "please fail this task" }
```

Mock 规则：prompt 含 `fail` 或 `失败` 会失败并重试。

---

## 十、文档 + 检索验收示例

1. `POST /documents` 上传 `.md` 文件
2. `GET /documents/{documentId}/chunks` 确认 chunks
3. `POST /documents/{documentId}/embeddings`
4. `POST /documents/search` 带 query
5. （可选）`POST /evaluations/retrieval` 传入 cases
6. （可选）`POST /rag/answer`

---

## 十一、常用 SQL

```sql
SELECT id, prompt, status, retry_count, llm_provider, llm_model
FROM task ORDER BY id DESC LIMIT 10;

SELECT task_id, event_type, from_status, to_status, message
FROM task_event WHERE task_id = ? ORDER BY created_at;

SELECT id, original_filename, status, chunk_count FROM document ORDER BY id DESC;

SELECT document_id, chunk_index, embedding_provider, embedding_model, vector_dimension
FROM document_chunk_embedding WHERE document_id = ?;
```

---

## 十二、注意事项

- 默认 embedding 为 mock，默认 VectorStore 为 exact。
- OpenAI / local-worker / Qdrant 需手工配置与启动，默认测试不依赖。
- `/dev` 与 `/dev/tasks` 下接口仅用于本地调试。
- Docker Compose 只管理 MySQL / RabbitMQ，Spring Boot 与 Python worker / Qdrant 需本机另行启动。
- 不要编造不存在的 HTTP API；benchmark 对比见测试 harness。
