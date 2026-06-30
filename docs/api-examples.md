# API 验收文档

## 一、文档目的

本文档用于记录 AI Task Orchestrator 当前阶段的 API 调用示例和验收方式，方便本地开发时验证任务创建、查询、状态流转、失败重试、幂等保护、取消任务和超时处理。

## 二、前置条件

验收前需要确认：

- Docker Compose 已启动 MySQL 和 RabbitMQ。
- Spring Boot 已使用 `docker` profile 启动。
- RabbitMQ 管理后台可访问。
- MySQL 可连接。
- Flyway 迁移已执行成功。

详细本地启动方式见：[docs/local-dev.md](local-dev.md)。

## 三、创建任务

接口：

```http
POST http://localhost:8080/tasks
Content-Type: application/json
```

请求体：

```json
{
  "prompt": "normal task"
}
```

说明：

创建任务后，任务初始状态为 `PENDING`，系统会发送 RabbitMQ 消息，Consumer 异步执行。

期望响应示例：

```json
{
  "taskId": 1,
  "status": "PENDING"
}
```

后续查询任务，正常情况下最终会变成 `SUCCESS`。

## 四、查询任务

接口：

```http
GET http://localhost:8080/tasks/{taskId}
```

说明：

查询任务详情。

响应字段至少包括：

- `id`
- `prompt`
- `status`
- `errorMessage`
- `retryCount`
- `maxRetry`
- `nextRetryAt`
- `timeoutSeconds`
- `timeoutAt`
- `createdAt`
- `updatedAt`

响应示例：

```json
{
  "id": 1,
  "prompt": "normal task",
  "status": "SUCCESS",
  "errorMessage": null,
  "retryCount": 0,
  "maxRetry": 3,
  "nextRetryAt": null,
  "timeoutSeconds": 30,
  "timeoutAt": "2026-06-30T12:00:30",
  "createdAt": "2026-06-30T12:00:00",
  "updatedAt": "2026-06-30T12:00:05"
}
```

## 五、手动修改任务状态

接口：

```http
PATCH http://localhost:8080/tasks/{taskId}/status
Content-Type: application/json
```

请求体示例：

```json
{
  "status": "CANCELLED",
  "message": "手动取消任务"
}
```

说明：

该接口用于测试状态机，不是主要业务入口。

状态机会阻止非法流转，例如：

- `SUCCESS -> RUNNING`
- `FAILED -> RUNNING`
- `CANCELLED -> RUNNING`

## 六、取消任务

接口：

```http
POST http://localhost:8080/tasks/{taskId}/cancel
```

说明：

支持取消以下状态的任务：

- `PENDING`
- `RETRY_PENDING`
- `RUNNING`

取消后状态变为 `CANCELLED`。

验收场景：

1. `PENDING` 任务取消，期望 `PENDING -> CANCELLED`。
2. `RETRY_PENDING` 任务取消，期望 `RETRY_PENDING -> CANCELLED`。
3. `RUNNING` 任务协作式取消，期望 `RUNNING -> CANCELLED`，执行线程检查到取消后不再写入成功或失败状态。
4. `SUCCESS` / `FAILED` 任务取消应返回 `400`。

## 七、开发测试重复投递接口

接口：

```http
POST http://localhost:8080/dev/tasks/{taskId}/dispatch
```

说明：

该接口仅用于本地开发测试，用来模拟 RabbitMQ 重复投递同一个 `taskId`，不是生产接口。

验收场景：

1. 对 `SUCCESS` 任务重复投递，任务不应重新执行。
2. 对 `FAILED` 任务重复投递，任务不应重新执行。
3. 对 `CANCELLED` 任务重复投递，任务不应重新执行。
4. `task_event` 不应出现 `SUCCESS -> RUNNING`、`FAILED -> RUNNING`、`CANCELLED -> RUNNING`。

## 八、正常任务验收

步骤：

1. 创建 `prompt = normal task` 的任务。
2. 等待几秒。
3. 查询任务。
4. 期望 `status = SUCCESS`。
5. 查询 `task_event`。

SQL：

```sql
SELECT task_id, event_type, from_status, to_status, message, created_at
FROM task_event
WHERE task_id = 你的任务ID
ORDER BY created_at;
```

期望事件：

- `TASK_CREATED`
- `PENDING -> RUNNING`
- `RUNNING -> SUCCESS`

## 九、失败重试验收

创建任务：

```json
{
  "prompt": "please fail this task"
}
```

说明：

当前模拟失败规则是 prompt 包含 `fail` 或 `失败` 时抛出异常。

期望流程：

- `PENDING -> RUNNING`
- `RUNNING -> RETRY_PENDING`
- `RETRY_PENDING -> RUNNING`
- 多次重试
- 最终 `RUNNING -> FAILED`

期望最终：

- `status = FAILED`
- `retryCount = maxRetry`
- `errorMessage` 不为空

## 十、重试成功验收

说明：

因为 prompt 包含 `fail` 会一直失败，所以需要手动在任务进入 `RETRY_PENDING` 后修改 prompt。

SQL 示例：

```sql
UPDATE task
SET prompt = 'retry success task'
WHERE id = 你的任务ID;
```

然后等待 Scheduler 重新投递。

期望：

- `RETRY_PENDING -> RUNNING`
- `RUNNING -> SUCCESS`

## 十一、取消任务验收

验收三个场景：

1. `PENDING -> CANCELLED`
2. `RETRY_PENDING -> CANCELLED`
3. `RUNNING -> CANCELLED`

取消后不应再变成：

- `SUCCESS`
- `FAILED`
- `RETRY_PENDING`

## 十二、超时任务验收

当前超时字段：

- `timeout_seconds`
- `timeout_at`

验收方式：

创建任务后，任务进入 `RUNNING` 时手动把 `timeout_at` 改为过去时间：

```sql
UPDATE task
SET timeout_at = NOW(6) - INTERVAL 1 SECOND
WHERE id = 你的任务ID
  AND status = 'RUNNING';
```

等待 TimeoutScheduler 扫描。

期望：

- `status = FAILED`
- `errorMessage = 任务执行超时`
- `task_event` 有 `RUNNING -> FAILED`

## 十三、常用 SQL

查询最近任务：

```sql
SELECT id, prompt, status, error_message, retry_count, max_retry, next_retry_at, timeout_seconds, timeout_at, created_at, updated_at
FROM task
ORDER BY id DESC;
```

查询某个任务事件：

```sql
SELECT task_id, event_type, from_status, to_status, message, created_at
FROM task_event
WHERE task_id = 你的任务ID
ORDER BY created_at;
```

查询 Flyway 版本：

```sql
SELECT version, description, script, success
FROM flyway_schema_history
ORDER BY installed_rank;
```

## 十四、当前 API 列表

- `POST /tasks`
- `GET /tasks/{taskId}`
- `PATCH /tasks/{taskId}/status`
- `POST /tasks/{taskId}/cancel`
- `POST /dev/tasks/{taskId}/dispatch`

## 十五、注意事项

- 当前项目尚未接入真实 LLM。
- 当前任务执行是模拟执行。
- `/dev` 开头接口只用于本地验收。
- 当前超时验收可以通过手动 SQL 修改 `timeout_at` 触发。
- 当前 Docker Compose 只管理 MySQL 和 RabbitMQ，Spring Boot 仍由本机启动。
