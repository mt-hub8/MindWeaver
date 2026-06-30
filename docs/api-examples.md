# API 验收文档

## 一、文档目的

本文档用于记录 AI Task Orchestrator 当前阶段的 API 调用示例和验收方式，重点覆盖 Prompt Template、Mock LLM、LLM metadata 与 token usage 记录。

## 二、前置条件

- Docker Compose 已启动 MySQL 和 RabbitMQ。
- Spring Boot 已使用 `docker` profile 启动。
- Flyway 迁移已执行成功。
- `prompt_template` 表中存在启用的 `default_task_prompt`。

详细本地启动方式见：[docs/local-dev.md](local-dev.md)。

## 三、创建任务

```http
POST http://localhost:8080/tasks
Content-Type: application/json
```

```json
{
  "prompt": "normal task"
}
```

创建任务后，Consumer 会查询 `default_task_prompt`，渲染 `renderedPrompt`，再调用 `MockLlmClient`。

## 四、查询任务

```http
GET http://localhost:8080/tasks/{taskId}
```

响应字段包括：

- `resultContent`
- `llmModel`
- `renderedPrompt`
- `promptTemplateCode`
- `llmProvider`
- `promptTokenCount`
- `completionTokenCount`
- `totalTokenCount`
- `llmLatencyMs`

示例：

```json
{
  "status": "SUCCESS",
  "llmModel": "mock-llm",
  "llmProvider": "mock",
  "promptTokenCount": 20,
  "completionTokenCount": 30,
  "totalTokenCount": 50,
  "llmLatencyMs": 3,
  "renderedPrompt": "你是一个任务执行助手，请根据用户输入完成任务。用户输入：normal task",
  "promptTemplateCode": "default_task_prompt",
  "resultContent": "Mock LLM response for prompt: 你是一个任务执行助手，请根据用户输入完成任务。用户输入：normal task"
}
```

## 五、正常任务验收

1. 创建 `prompt = normal task` 的任务。
2. 等待任务完成。
3. 查询任务详情。

期望：

- `status = SUCCESS`
- `llmProvider = mock`
- `llmModel = mock-llm`
- `promptTokenCount` 不为空
- `completionTokenCount` 不为空
- `totalTokenCount` 不为空
- `llmLatencyMs` 不为空
- `resultContent` 不为空
- `renderedPrompt` 不为空
- `promptTemplateCode = default_task_prompt`

SQL：

```sql
SELECT id, status, llm_provider, llm_model,
       prompt_token_count, completion_token_count, total_token_count, llm_latency_ms,
       rendered_prompt, prompt_template_code, result_content, error_message
FROM task
WHERE id = 你的任务ID;
```

## 六、失败任务验收

创建任务：

```json
{
  "prompt": "please fail this task"
}
```

期望：

- 渲染后的 Prompt 仍包含 `fail`
- 任务失败并进入重试
- 重试耗尽后 `FAILED`
- 失败任务也能看到 `promptTokenCount`、`totalTokenCount`、`llmLatencyMs`
- `completionTokenCount = 0`
- `errorMessage` 包含 `Mock LLM execution failed`

## 七、常用 SQL

查询最近任务：

```sql
SELECT id, prompt, status, error_message,
       llm_provider, llm_model,
       prompt_token_count, completion_token_count, total_token_count, llm_latency_ms,
       rendered_prompt, prompt_template_code,
       result_content, created_at, updated_at
FROM task
ORDER BY id DESC;
```

查询事件：

```sql
SELECT task_id, event_type, from_status, to_status, message, created_at
FROM task_event
WHERE task_id = 你的任务ID
ORDER BY created_at;
```

查询 Flyway：

```sql
SELECT version, description, script, success
FROM flyway_schema_history
ORDER BY installed_rank;
```

## 八、注意事项

- 当前 token usage 是 Mock 估算，不是真实 tokenizer。
- 当前没有真实成本统计。
- 当前没有接入真实 OpenAI / Claude / 本地模型 Provider。
- 当前没有 Streaming / RAG / Agent / KV Cache。
- 当前没有 Prompt Template CRUD API。
