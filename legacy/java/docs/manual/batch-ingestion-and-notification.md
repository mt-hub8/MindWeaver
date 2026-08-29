# V13.0 批量文档导入与通知中心

## 目标

V13.0 在本地个人知识库场景下，提供**可控、可恢复、可观察**的批量文档导入，以及**站内通知中心**。

不做：高并发云端批处理、邮件通知、系统托盘通知、WebSocket/SSE、语义近似去重。

## 为什么需要 Batch / Item 模型

批量上传不是简单的 multiple file upload，而是三层结构：

- `upload_batch`：批次汇总（总进度、状态、统计）
- `upload_batch_item`：单文件状态（去重、失败、重试、staging）
- `document_ingestion_task`：复用已有异步摄入流水线

这样可以在批次级别观察进度，在文件级别重试/取消，且不重复实现 chunk / embedding 逻辑。

## 批量导入生命周期

1. 用户一次选择多个文件 → `POST /documents/batches/upload`
2. 创建 `upload_batch` 与 `upload_batch_item`
3. 计算 `fileHash`，保存 staging 文件
4. 文件级去重（按 `duplicatePolicy`）
5. 非跳过文件进入 `DocumentIngestionService.submitIngestion`
6. MQ 异步处理：chunk →（文本级去重）→ embedding → vector write
7. item / batch 状态汇总更新
8. 批次到达终态时生成站内通知

## Item 状态说明

| 状态 | 含义 |
|------|------|
| PENDING | 待进入队列 |
| QUEUED | 已提交摄入任务 |
| PROCESSING | 摄入处理中 |
| COMPLETED | 成功 |
| FAILED | 失败，可重试 |
| SKIPPED_DUPLICATE_FILE | 文件级重复跳过 |
| SKIPPED_DUPLICATE_TEXT | 文本级重复跳过 |
| CANCEL_REQUESTED | 正在取消 |
| CANCELED | 已取消 |

## 文件级去重

`fileHash = SHA-256(file bytes)`

- 若存在 **ACTIVE** 文档相同 hash：
  - `SKIP`：标记 `SKIPPED_DUPLICATE_FILE`，不创建 document
  - `USE_EXISTING`：关联已有文档，可加入目标 Collection
  - `IMPORT_ANYWAY`：仍创建新 document，记录警告
- 若重复文档在 **TRASHED**：不自动恢复，提示先恢复或重新导入
- 若已 **PURGED**：视为不存在，允许重新导入

## 文本级去重

解析后：`textHash = SHA-256(normalized text)`

若不同文件但文本相同，默认跳过 embedding，标记 `SKIPPED_DUPLICATE_TEXT`。

## Chunk 级 embedding cache 复用

chunk 级不去重新实现，继续复用 `EmbeddingCacheService`：

`chunkHash + provider + model + dimension`

## 为什么不做语义近似去重

V13 禁止 SimHash / MinHash / embedding similarity dedup，避免误跳过或误合并。

## 并发控制策略

配置项（`application.properties`）：

```properties
app.batch-ingestion.max-files-per-batch=100
app.batch-ingestion.document-parse-concurrency=2
app.batch-ingestion.embedding-concurrency=1
app.batch-ingestion.max-retry-count=2
app.batch-ingestion.staging-dir=data/staging/batches
```

第一版复用现有 MQ 摄入队列；runner 控制同时提交到队列的 pending 数量。

## 失败重试规则

`POST /documents/batches/{batchId}/retry-failed`

- 仅重试 `FAILED` item
- 不重试 SKIPPED / COMPLETED / CANCELED
- `retryCount` 超过 `max-retry-count` 不再重试
- 复用 `staging_file_path`；文件丢失则返回清晰错误

## 取消规则

`POST /documents/batches/{batchId}/cancel`

- batch → `CANCEL_REQUESTED`
- `PENDING` / `QUEUED` item → `CANCELED`
- `PROCESSING` item 不强杀
- **不删除**已成功导入的文档

## Staging Area

失败重试依赖磁盘 staging：`data/staging/batches/{batchId}/{itemId}_{filename}`

## 通知中心

表 `notification`，API：

- `GET /notifications`
- `GET /notifications/unread-count`
- `POST /notifications/{id}/read`
- `POST /notifications/read-all`

批次终态映射：

| Batch 状态 | 通知类型 |
|------------|----------|
| COMPLETED | BATCH_COMPLETED |
| PARTIAL_FAILED | BATCH_PARTIAL_FAILED |
| FAILED | BATCH_FAILED |
| CANCELED | BATCH_CANCELED |

`target_type=UPLOAD_BATCH`，`target_id=batchId`，前端跳转 `/batch-ingestion.html?batchId=...`

## 为什么 V13 不做邮件通知

个人本地工作台第一版优先站内可观察性；邮件需要账户体系与外部依赖，超出本阶段范围。

## 默认低依赖测试

`mvn test` 不依赖 Ollama / Python Worker / Qdrant / Docker / 真实 MQ 消费；集成测试 mock `DocumentIngestionMessagePublisher`。

## 常见问题 FAQ

**Q: 单文件上传还能用吗？**  
A: 可以，`POST /documents/upload` 未改变。

**Q: 取消批次会删除已导入文档吗？**  
A: 不会，仅停止剩余任务。

**Q: 垃圾箱里的重复文件会自动恢复吗？**  
A: 不会，需手动恢复或选择 `IMPORT_ANYWAY`。

**Q: 如何查看批次配置？**  
A: 系统设置页只读展示，或 `GET /documents/batches/config`。

## 后续路线

V14.0 可选：Local App Packaging 或 Skill System MVP（见 README 路线图）。
