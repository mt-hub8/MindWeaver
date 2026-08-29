# RAG Demo Golden Path（V2.7.2）

本文提供一条**稳定、可复现**的手工黄金路径：

```text
启动依赖 → 上传 demo 文档 → 生成 embedding → 打开 /rag-demo.html → 输入推荐问题 → 查看 answer / citations / metadata
```

本路径基于仓库**真实存在**的 API 与配置，不依赖一键 seed、ingestion admin API 或前端构建工具。

---

## 1. 目标

| 项 | 说明 |
| --- | --- |
| 演示入口 | 浏览器打开 `http://localhost:8080/rag-demo.html` |
| 后端接口 | `POST /rag/answers`（V2.7 已实现，本版本不修改） |
| Demo 语料 | `docs/demo/rag-demo-source.md` |
| 摄入方式 | 现有 `POST /documents` + `POST /documents/{documentId}/embeddings` |
| 推荐栈 | MySQL/RabbitMQ + Qdrant + Local Embedding Worker + Spring Boot |

成功标准：

- Web UI 可加载；
- 推荐问题能返回非 no-context 的 answer；
- `citations` 至少 1 条，且含 `sourceIndex` / `documentId` / `chunkId` / `score` / `contentSnippet`；
- `retrieval` 显示当前 provider、model、dimension、vectorStore；
- `generation` 显示 Mock LLM 的 provider/model，或 no-context 时的 `skipped` / `reason`。

---

## 2. 前置条件

- JDK 21+
- Docker Desktop（MySQL、RabbitMQ、Qdrant）
- Python 3.10+（Local Embedding Worker）
- PowerShell
- IDE 可选：支持 `.http` 文件（IntelliJ / VS Code REST Client）

进入项目目录：

```powershell
cd E:\code\ai-task-orchestrator
```

---

## 3. 仓库已有摄入能力与限制

### 3.1 已发现的 API / 文档 / HTTP 文件

| 类型 | 路径 | 用途 |
| --- | --- | --- |
| 文档上传 | `POST /documents` | multipart 参数 `file`，仅 `.txt` / `.md` |
| 文档查询 | `GET /documents/{documentId}` | 元信息 |
| Chunk 查询 | `GET /documents/{documentId}/chunks` | 切分结果 |
| 生成 embedding | `POST /documents/{documentId}/embeddings` | 写入 VectorStore |
| 语义检索 | `POST /documents/search` | 验证检索（可选） |
| RAG 问答 | `POST /rag/answers` | query + topK |
| Web UI | `GET /rag-demo.html` | 极简演示页 |
| 已有 HTTP | `docs/demo/rag-flow.http` | mock + exact 基线 |
| 已有 HTTP | `docs/demo/qdrant-flow.http` | Qdrant + mock embedding |
| 本版本 HTTP | `docs/demo/rag-demo-golden-path.http` | 本黄金路径 |
| 本地开发 | `docs/local-dev.md` | 基础设施与 profile |
| Qdrant 验证 | `docs/manual/qdrant-verification.md` | Qdrant profile 说明 |
| Worker | `workers/embedding-worker/README.md` | Python worker 启动 |

### 3.2 明确限制（必须知晓）

- **没有** demo seed API、ingestion admin API、一键导入或自动清理接口。
- 摄入只能走：`上传文件 → chunking（自动）→ embed（手工调用 API）`。
- RAG 默认使用 **Mock LLM**，不代表生产生成质量。
- `POST /rag/answers` 的 query embedding **不经过** Embedding Cache。
- 切换 embedding provider 或 dimension 后，必须**重新 embed**，且 Qdrant collection 不能混用不同向量空间。
- 本黄金路径**不是**默认 `mvn test` 依赖，需要本机 Docker、网络（首次下载模型）与手工启动服务。

---

## 4. 启动 MySQL / RabbitMQ

```powershell
cd E:\code\ai-task-orchestrator
docker compose up -d
docker compose ps
```

| 服务 | 本机端口 |
| --- | --- |
| MySQL | `3307` |
| RabbitMQ AMQP | `5672` |
| RabbitMQ 管理台 | `15672`（guest / guest） |

---

## 5. 启动 Qdrant

使用独立 compose 文件（不修改主 `docker-compose.yml`）：

```powershell
cd E:\code\ai-task-orchestrator
docker compose -f docker-compose.qdrant.yml up -d
docker compose -f docker-compose.qdrant.yml ps
```

健康检查：

```powershell
Invoke-WebRequest -Uri http://127.0.0.1:6333/healthz -UseBasicParsing
```

| 项 | 值 |
| --- | --- |
| HTTP API | `http://127.0.0.1:6333` |
| Collection（默认） | `ai_task_orchestrator_chunks` |
| 本路径 embedding dimension | **384**（local-worker） |

> 若该 collection 曾用 mock embedding（128 维）写入，请先清空数据卷后重试：
>
> ```powershell
> docker compose -f docker-compose.qdrant.yml down -v
> docker compose -f docker-compose.qdrant.yml up -d
> ```

---

## 6. 启动 Local Embedding Worker

新开 PowerShell 窗口：

```powershell
cd E:\code\ai-task-orchestrator\workers\embedding-worker
python -m venv .venv
.\.venv\Scripts\Activate.ps1
pip install -r requirements.txt
uvicorn main:app --host 127.0.0.1 --port 8001
```

健康检查：

```powershell
Invoke-WebRequest -Uri http://127.0.0.1:8001/health -UseBasicParsing
```

注意：

- 默认模型 `sentence-transformers/all-MiniLM-L6-v2`，dimension **384**；
- **首次 embed 可能较慢**（需从 Hugging Face 下载模型）；
- 保持该终端运行，勿关闭。

---

## 7. 启动 Spring Boot

再开一个 PowerShell 窗口。必须同时启用 `docker` 与 `qdrant` profile，并将 embedding provider 覆盖为 `local-worker`：

```powershell
cd E:\code\ai-task-orchestrator
.\mvnw.cmd spring-boot:run "-Dspring-boot.run.profiles=docker,qdrant" "-Dspring-boot.run.arguments=--app.embedding.provider=local-worker"
```

生效后的关键配置：

| 配置项 | 值 |
| --- | --- |
| `app.embedding.provider` | `local-worker` |
| `app.embedding.local-worker.base-url` | `http://127.0.0.1:8001` |
| `app.embedding.local-worker.model` | `sentence-transformers/all-MiniLM-L6-v2` |
| `app.embedding.local-worker.dimension` | `384` |
| `app.vector-store.provider` | `qdrant`（来自 `application-qdrant.properties`） |
| LLM | 默认 Mock（`mock` / `mock-llm`） |

常见 profile 错误：

- 只启 `docker` 未启 `qdrant` → VectorStore 仍为 `exact`，与 Qdrant 无关；
- 只启 `qdrant` 未启 `docker` → MySQL 端口可能连错；
- 未覆盖 `app.embedding.provider=local-worker` → 仍用 mock（128 维），与 Qdrant collection 期望的 384 维冲突。

### 7.1 快速验证路径（可选，非本黄金路径主栈）

若暂时不想启动 Qdrant / Worker，可用 mock + exact 做 smoke test：

```powershell
.\mvnw.cmd spring-boot:run "-Dspring-boot.run.profiles=docker"
```

此时 `retrieval.vectorStore` 为 `ExactCosineVectorStore`，`dimension=128`。仍可完成上传与 RAG，但**不符合**本文推荐的 Qdrant + local-worker 演示栈。

---

## 8. Demo Source 文档路径

```text
docs/demo/rag-demo-source.md
```

该文件覆盖：项目概述、RAG 流程、citations、Embedding Cache、Minimal Web UI 设计取舍、Qdrant Benchmark 意义等主题，并按 Markdown 标题切分为多个 chunk。

---

## 9. 摄入 Demo 文档

### 方式 A：使用 HTTP 文件（推荐）

在 IDE 中打开 `docs/demo/rag-demo-golden-path.http`，按顺序执行：

1. 上传 `docs/demo/rag-demo-source.md`
2. `GET /documents/{documentId}/chunks`
3. `POST /documents/{documentId}/embeddings`
4. （可选）`POST /documents/search` 预检检索
5. `POST /rag/answers` 预检 API
6. 浏览器打开 `/rag-demo.html`

### 方式 B：PowerShell + curl

上传：

```powershell
cd E:\code\ai-task-orchestrator
curl.exe -X POST http://localhost:8080/documents -F "file=@docs/demo/rag-demo-source.md"
```

记录响应中的 `documentId`（示例假设为 `1`），然后：

```powershell
curl.exe http://localhost:8080/documents/1/chunks
curl.exe -X POST http://localhost:8080/documents/1/embeddings
```

期望：

- 上传后 `status = CHUNKED`，`chunkCount > 0`；
- embed 响应中 `embeddingProvider = local-worker`，`dimension = 384`；
- 无 `VECTOR_STORE_ERROR` 或 worker 连接错误。

可选检索预检：

```powershell
curl.exe -X POST http://localhost:8080/documents/search -H "Content-Type: application/json" -d "{\"query\":\"RAG Answer API 的核心流程是什么\",\"topK\":5}"
```

若 search 有结果而 RAG 仍 no-context，优先检查是否忘记 embed 或 provider/dimension 不一致。

---

## 10. 打开 Web UI

浏览器访问：

```text
http://localhost:8080/rag-demo.html
```

页面使用同源相对路径调用 `POST /rag/answers`，无需额外 CORS 配置。

操作：

1. 在 Query 输入推荐问题（见下一节）；
2. Top K 保持默认 `5`（范围 1–10）；
3. 点击 **Ask**；
4. 观察 loading、answer、citations、retrieval / generation metadata。

---

## 11. 推荐问题与预期现象

以下问题均可在 `docs/demo/rag-demo-source.md` 中找到相关段落。实际 `chunkId`、`score` 因库中数据而异，以运行时响应为准。

### 11.1 AI Task Orchestrator 的目标是什么？

**预期 answer（Mock LLM）**：包含类似「根据检索到的上下文，问题与以下来源相关：[1] …」的句式（具体编号取决于命中 chunk 数）。

**预期 citations**：≥1 条；`contentSnippet` 应提及异步任务编排、LLM 抽象、RAG 闭环等关键词。

**预期 retrieval**：

```json
{
  "topK": 5,
  "returned": 1,
  "provider": "local-worker",
  "model": "sentence-transformers/all-MiniLM-L6-v2",
  "dimension": 384,
  "vectorStore": "QdrantVectorStore"
}
```

`returned` 可能为 1–5，取决于相似度与 topK。

**预期 generation**：

```json
{
  "provider": "mock",
  "model": "mock-llm",
  "skipped": null,
  "reason": null
}
```

### 11.2 RAG Answer API 的核心流程是什么？

**预期**：citations 命中 `rag-answer-pipeline` 相关段落；snippet 含 query embedding、VectorStore search、citations、RagPromptBuilder、LlmClient 等词。

**generation**：`skipped` 不为 `true`。

### 11.3 citations 在 RAG 系统中有什么作用？

**预期**：citations 含 `sourceIndex`、`documentId`、`chunkId`、`score`、`contentSnippet`；snippet 提及可追溯性、chunk-level citation。

### 11.4 Embedding Cache 使用什么作为 cache key？

**预期**：snippet 明确出现 **chunkHash + provider + model + dimension** 四元组；并说明不能只用 chunkHash。

**说明**：此问题验证的是 demo 文档内容；RAG query 本身不走 cache，但 `GET /embedding-cache/metrics` 可在 ingest 后观察 document embedding 的 cache 行为。

### 11.5 为什么 Minimal Web UI 没有引入 React 或 Vue？

**预期**：answer 基于 minimal-web-ui 段落；snippet 提及 static resources、零构建、不做多轮对话等。

### 11.6（可选）Qdrant Benchmark 的目的是什么？

**预期**：snippet 提及 ExactCosine vs Qdrant、手工 opt-in、`build/reports/retrieval/`、不能声称「Qdrant 一定更快」等。

---

## 12. 空 citations / no-context 时的预期

若**未摄入**或**未 embed** demo 文档：

| 字段 | 现象 |
| --- | --- |
| `answer` | `根据当前检索到的文档内容，无法确定。` |
| `citations` | `[]`（Web UI 显示「暂无引用来源」） |
| `retrieval.returned` | `0` |
| `generation.skipped` | `true` |
| `generation.reason` | `NO_RETRIEVED_CONTEXT` |

这是 V2.7 设计的 no-context 路径，不是页面 bug。

---

## 13. 常见失败排查

### 13.1 Qdrant 未启动

**现象**：`POST /documents/{id}/embeddings` 或 `/rag/answers` 返回 `VECTOR_STORE_ERROR`；日志含连接 `6333` 失败。

**处理**：

```powershell
docker compose -f docker-compose.qdrant.yml up -d
Invoke-WebRequest -Uri http://127.0.0.1:6333/healthz -UseBasicParsing
```

确认 Spring 启用了 `qdrant` profile。

### 13.2 Local Embedding Worker 未启动

**现象**：embed 或 RAG 报 embedding / LLM provider 相关错误；worker 连接超时。

**处理**：按第 6 节启动 worker，确认 `http://127.0.0.1:8001/health` 返回 200。

### 13.3 Spring profile 使用错误

| 错误 | 后果 |
| --- | --- |
| 未启 `docker` | MySQL 端口错误 |
| 未启 `qdrant` | 仍用 ExactCosine，Qdrant 无数据 |
| 未设 `local-worker` | mock 128 维与 Qdrant 384 维 collection 冲突 |

**处理**：使用第 7 节完整启动命令。

### 13.4 embedding dimension 不匹配

**现象**：Qdrant upsert 失败；或 collection `size` 与 embedding 不一致。

**处理**：

- local-worker 路径必须为 **384**；
- 清空 Qdrant 卷后重建 collection；
- 重新 `POST /documents/{documentId}/embeddings`。

检查 collection：

```powershell
Invoke-WebRequest -Uri http://127.0.0.1:6333/collections/ai_task_orchestrator_chunks -UseBasicParsing
```

### 13.5 没有成功摄入 demo 数据

**现象**：RAG 一直 no-context；`retrieval.returned = 0`。

**检查清单**：

1. 是否执行 `POST /documents` 且 `chunkCount > 0`；
2. 是否对**同一** `documentId` 执行 `POST /documents/{documentId}/embeddings`；
3. embed 是否成功（无 5xx）；
4. 当前 active provider 是否与 embed 时一致。

### 13.6 页面返回 no-context answer

同 13.5。另检查 query 是否与 demo 文档语言/主题相关；可先用 `POST /documents/search` 验证检索。

### 13.7 citations 为空但 answer 有内容

在 no-context 路径下 answer 为固定文案且 `generation.skipped=true`，此时 citations 必为空。若 `skipped=false` 仍无 citations，属异常，检查 API 响应 JSON 与后端日志。

### 13.8 `/rag-demo.html` 能打开但 `/rag/answers` 失败

| 可能原因 | 处理 |
| --- | --- |
| Spring Boot 未启动 | 启动应用 |
| 请求校验失败（空 query） | 填写 query |
| 后端 5xx（DB / Qdrant / worker） | 看响应 `code` / `message` / `traceId` 与服务器日志 |
| 网络/端口错误 | 确认访问的是 `localhost:8080` 同源 |

Web UI 会在 error 区展示 `code`、`message`、`traceId`；网络错误显示 `NETWORK_ERROR` fallback。

---

## 14. 相关文件

- Demo 语料：[`docs/demo/rag-demo-source.md`](../demo/rag-demo-source.md)
- HTTP 步骤：[`docs/demo/rag-demo-golden-path.http`](../demo/rag-demo-golden-path.http)
- 本地开发：[`docs/local-dev.md`](../local-dev.md)
- Qdrant 验证：[`docs/manual/qdrant-verification.md`](qdrant-verification.md)
- Qdrant Benchmark：[`docs/manual/qdrant-benchmark-result-capture.md`](qdrant-benchmark-result-capture.md)
- Worker：[`workers/embedding-worker/README.md`](../../workers/embedding-worker/README.md)
- Web UI 静态资源：`src/main/resources/static/rag-demo.html`

---

## 15. 本版本明确不做

- 一键导入 / demo seed API / 自动清理
- 修改 RAG API、Web UI、Worker、Qdrant 核心逻辑
- 前端框架或构建系统
- streaming、多轮对话、文档管理页面
