# V6.0 真实 AI Runtime 接入与 Agent 任务编排

## 目标

V6.0 将项目从「知识库问答系统」推进为「**AI 任务编排系统**」：

- 用户提交 **AI 任务**（标题 + 目标 + 可选知识库分组）；
- 系统**异步**执行：检索知识库 → 调用大模型 → 保存结果、引用、执行过程与模型 metadata；
- 同时接入 **Java + Python** 的真实 AI Runtime（默认仍为 mock，测试不依赖真实模型）。

## Java + Python 分工

| 层级 | 职责 |
|------|------|
| **Java / Spring Boot** | API、任务状态、MySQL、RabbitMQ、RAG 编排、citations、Collection 范围、生命周期过滤、事件时间线、Web UI |
| **Python Worker** | Embedding 向量计算、LLM 文本生成（`local-ai` profile） |

## Python AI Runtime Worker

目录：`workers/ai-runtime-worker/`

| 接口 | 说明 |
|------|------|
| `GET /health` | 健康检查 |
| `POST /embed` | 批量 embedding |
| `POST /embeddings` | 兼容现有 Java Local Worker 客户端 |
| `POST /generate` | LLM 生成（systemPrompt + userPrompt） |

### 启动 Worker（手动）

```bash
cd workers/ai-runtime-worker
python -m venv .venv
.venv\Scripts\activate
pip install -r requirements.txt
uvicorn main:app --host 127.0.0.1 --port 8001
```

首次运行会下载 embedding 模型（需网络），CPU 可运行，适合本地开发。

### 启动 Spring Boot（local-ai profile）

```bash
.\mvnw.cmd spring-boot:run -Dspring-boot.run.profiles=local-ai
```

`application-local-ai.properties` 会将：

- `app.embedding.provider=local-worker` → `http://127.0.0.1:8001`
- `app.llm.provider=local-python` → `http://127.0.0.1:8001/generate`

## 默认 Mock Provider

`application.properties` 默认：

- `app.embedding.provider=mock`
- `app.llm.provider=mock`

**默认 `mvn test` 不启动 Python worker、不下载模型、不调用外部 API。**

## 如何创建 Agent Task

### Web UI

打开 [`/agent-tasks.html`](/agent-tasks.html)：

1. 填写**任务标题**、**任务目标**；
2. 选择**知识库范围**（全部文档 / 指定知识库分组）；
3. 点击**提交任务**；
4. 查看**任务结果**、**引用来源**、**执行过程**、**模型调用信息**。

### API

`POST /agent/tasks`（202 Accepted）

```json
{
  "title": "总结项目 A 文档",
  "objective": "基于项目 A 文档，总结系统核心功能、风险点和下一步建议。",
  "collectionId": 1
}
```

- `collectionId` 可选；不传表示**全部文档**。

查询：

- `GET /agent/tasks` — 任务列表
- `GET /agent/tasks/{taskId}` — 详情（结果、citations、metadata）
- `GET /agent/tasks/{taskId}/events` — 执行过程时间线

## Agent Task 状态

| 状态 | 中文展示 |
|------|----------|
| PENDING | 待处理 |
| RUNNING | 执行中 |
| COMPLETED | 已完成 |
| FAILED | 执行失败 |

## 执行流程

```text
创建任务 (PENDING)
→ 入队 RabbitMQ
→ 消费开始 (RUNNING)
→ 检索相关知识库 (RETRIEVAL_STARTED / COMPLETED)
→ 调用大模型 (LLM_STARTED / COMPLETED)
→ 保存结果与引用
→ 完成 (COMPLETED)
```

失败时：`FAILED` + 错误码 + 追踪 ID + `TASK_FAILED` 事件。

## Collection 范围如何生效

与 V5.0 一致：

- 不传 `collectionId`：全库检索（仍过滤已删除文档与旧片段）；
- 传 `collectionId`：仅在该分组内检索。

## V4.0 生命周期过滤

继续生效：已删除文档、非当前 generation 的 chunk 不会进入 citations 与 LLM 上下文。

## No-Context 行为

当前范围内没有可用知识库内容时：

- **不调用 LLM**；
- 任务状态为 **COMPLETED**（非 FAILED）；
- `result` = 「当前范围内没有可用于执行任务的知识库内容。」；
- `citationCount = 0`。

## LLM Failure 行为

大模型调用失败时：

- 任务状态 **FAILED**；
- 记录 `errorCode` / `errorMessage` / `traceId`；
- 事件时间线包含「任务执行失败，请查看错误原因。」

## 默认测试为什么不依赖真实模型

- 单元测试 mock `LlmProvider`、`AgentTaskMessagePublisher`、`RagTwoStageRetrievalService` 等；
- Spring Boot 测试使用 H2/嵌入式策略与 mock provider；
- 真实 Python worker 仅通过 `local-ai` profile 手动验证。

## 当前明确不做的边界

- 多 Agent / 复杂 Planner / Tool Calling / ReAct
- Streaming / WebSocket / SSE
- 用户登录 / 权限 / workspace / 多租户
- 长期对话记忆 / Agent Memory
- 代码执行沙箱 / 浏览器自动化

## 相关页面

- AI 任务编排：[`/agent-tasks.html`](/agent-tasks.html)
- 知识库问答：[`/ask.html`](/ask.html)
- 知识库分组：[`/collections.html`](/collections.html)
