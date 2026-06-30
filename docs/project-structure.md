# 项目结构说明

## 一、文档目的

本文档说明项目结构、核心包职责，以及任务从创建到 Prompt 渲染、Mock LLM 调用、usage 保存和状态流转的链路。

## 二、整体目录结构

```text
src/main/java/com/tuoman/ai_task_orchestrator
├── config
├── controller
├── dto
├── entity
├── enums
├── llm
├── mq
├── prompt
├── repository
├── scheduler
├── service
├── state
└── AiTaskOrchestratorApplication.java
```

## 三、核心包职责

- `controller`：HTTP 接口。
- `dto`：请求和响应对象，`TaskDetailResponse` 返回 LLM 结果、Prompt 渲染信息和 usage 信息。
- `entity`：JPA 实体，`TaskEntity` 映射 task 表。
- `repository`：数据库访问，包含 `TaskRepository`、`TaskEventRepository`、`PromptTemplateRepository`。
- `mq`：RabbitMQ 消息生产和消费。
- `prompt`：`PromptTemplateRenderer`，负责渲染 `{{prompt}}`、`{{taskId}}`、`{{model}}`。
- `llm`：`LlmClient` 抽象、`LlmRequest`、`LlmResponse`、`MockLlmClient`。
- `service`：核心业务逻辑。
- `scheduler`：重试和超时扫描。
- `state`：任务状态机。

## 四、TaskEntity 当前关键字段

- `prompt`
- `status`
- `errorMessage`
- `retryCount` / `maxRetry` / `nextRetryAt`
- `timeoutSeconds` / `timeoutAt`
- `resultContent`
- `llmModel`
- `renderedPrompt`
- `promptTemplateCode`
- `llmProvider`
- `promptTokenCount`
- `completionTokenCount`
- `totalTokenCount`
- `llmLatencyMs`

## 五、llm 包

- `LlmClient`：统一 LLM 调用接口。
- `LlmRequest`：包含 `taskId`、`prompt`、`model`。当前 `prompt` 使用 `renderedPrompt`。
- `LlmResponse`：包含结果、错误信息、provider、token usage 和 latency。
- `MockLlmClient`：模拟 LLM Provider，不调用外部 API；provider 固定为 `mock`，token usage 基于字符串长度估算。

## 六、service 包

`TaskExecutionService`：

- 做入口幂等保护。
- 查询 `default_task_prompt`。
- 使用 `PromptTemplateRenderer` 渲染 `renderedPrompt`。
- 构造 `LlmRequest`。
- 调用 `LlmClient.generate(...)`。
- 调用 `TaskService.saveLlmMetadata(...)` 保存 provider / token usage / latency。
- 成功后调用 `markTaskSucceeded(...)` 保存结果和状态。
- 失败后进入重试或最终失败。
- 保留取消和超时防覆盖逻辑。

`TaskService`：

- 创建任务、查询任务、状态流转。
- 记录 `task_event`。
- 标记成功、失败、重试、取消、超时。
- 保存 LLM metadata，但不为 metadata 单独写事件。

## 七、迁移脚本

- `V1__create_task_table.sql`
- `V2__create_task_event_table.sql`
- `V3__add_error_message_to_task.sql`
- `V4__add_retry_fields_to_task.sql`
- `V5__add_timeout_fields_to_task.sql`
- `V6__add_llm_result_fields_to_task.sql`
- `V7__create_prompt_template_table.sql`
- `V8__add_prompt_render_fields_to_task.sql`
- `V9__add_llm_usage_fields_to_task.sql`

## 八、完整执行链路

```text
POST /tasks
-> task PENDING
-> RabbitMQ message
-> Consumer
-> tryStartTaskExecution
-> RUNNING
-> 查询 default_task_prompt
-> 渲染 renderedPrompt
-> LlmRequest.prompt = renderedPrompt
-> MockLlmClient.generate
-> 返回 provider / token usage / latency
-> 保存 LLM metadata
-> 成功保存 resultContent / llmModel / renderedPrompt / promptTemplateCode
-> RUNNING -> SUCCESS
```

失败时进入 `RETRY_PENDING`，重试耗尽后进入 `FAILED`。

## 九、当前边界

当前尚未实现：

- 真实 OpenAI / Claude / 本地模型 Provider
- API Key 配置
- 真实 tokenizer
- 真实成本统计
- Prompt Template CRUD API
- Streaming Output
- RAG
- Tool Calling
- Agent Runtime
- KV Cache-aware Scheduling
