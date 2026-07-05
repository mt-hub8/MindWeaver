# 垃圾箱与本地存储管理（V12.0）

## 目标

V12.0 将「删除文档」升级为产品级**垃圾箱与本地存储管理**：

- **ACTIVE** → **TRASHED** → **PURGED**
- 7 天内可恢复，无需重新索引
- 永久删除清理原始文本、片段、向量与缓存
- 系统设置展示存储统计与缓存管理

## 为什么需要垃圾箱

直接删除容易造成误操作且无法恢复。垃圾箱提供：

1. **可恢复期**：7 天内可一键恢复；
2. **即时生效**：进入垃圾箱后立即不参与问答、检索、Agent；
3. **数据保留**：恢复时无需重新 embedding 或重新索引；
4. **明确终态**：永久删除（PURGED）才清理底层数据。

## 生命周期

| 状态 | 含义 | 可用于问答 |
|------|------|------------|
| **ACTIVE** | 正常启用 | 是 |
| **TRASHED** | 已放入垃圾箱 | 否 |
| **PURGED** | 已永久删除 | 否 |

## 删除、恢复、永久删除

### 删除（进入垃圾箱）

`DELETE /documents/{documentId}`

- ACTIVE → TRASHED
- 设置 `trashedAt`、`purgeAfter = trashedAt + 7 天`
- **不**物理删除 chunks、vectors、缓存

### 恢复

`POST /documents/{documentId}/restore`

- TRASHED → ACTIVE
- 恢复后立即可用于 Ask / Retrieval / Agent
- **不需要**重新 embedding 或重新索引

### 永久删除

`POST /documents/{documentId}/purge`

- 仅允许 TRASHED 状态（ACTIVE 不可直接 purge）
- 清理：chunks、vectors、embedding cache（按 chunk hash）、source_text、分组映射
- 标记 PURGED，不可恢复

## 7 天自动清理

配置项（`application.properties`）：

```properties
app.trash.retention-days=7
app.trash.cleanup-enabled=true
app.trash.cleanup-cron=0 0 3 * * *
```

每天凌晨 3 点（可配置）清理 `purgeAfter < now` 的 TRASHED 文档。也可手动触发：

`POST /documents/trash/purge-expired`

## 为什么 TRASHED 恢复不需要重新索引

进入垃圾箱时仅变更生命周期状态，**不删除** chunks 与 vectors。恢复后过滤条件重新允许该文档，检索链路即可复用已有索引。

## 为什么 PURGED 要清理多项数据

永久删除表示用户明确放弃该文档。需清理：

- 解析文本（`source_text`）
- 文本片段（chunks）
- 向量索引（ExactCosine / Qdrant）
- 相关 embedding cache（按内容 hash）
- 知识库分组映射

避免残留数据占用空间或误检索。

## 本地数据目录规划

建议 Windows 目录：`%APPDATA%\PersonalAIKnowledgeWorkspace\`

当前开发模式仍使用项目配置的 MySQL、向量库与 Worker；详见系统设置页。

## 缓存 vs 垃圾箱

| | 垃圾箱 | 缓存清理 |
|---|--------|----------|
| 影响文档 | 是（生命周期） | 否 |
| 可恢复 | TRASHED 可恢复 | 缓存可自动重建 |
| 用途 | 用户删除意图 | 释放空间、排查问题 |

**清理缓存不会删除原始文档和知识库记录**，但可能导致下次处理变慢。

## 存储统计

`GET /storage/summary` 返回：

- 原始文件 / 解析文本占用（基于 ACTIVE 文档）
- chunk / vector / embedding cache 记录数
- Qdrant 向量可能显示「由 Qdrant 管理」

无法准确统计时不伪造数值。

## 缓存管理 API

- `POST /storage/cache/embedding/clear`
- `POST /storage/cache/retrieval/clear`（当前版本未启用 retrieval cache）
- `POST /storage/cache/clear-all`

## 当前不做

- 云同步 / 登录 / 多用户
- S3 / OSS / NAS
- 版本历史 / 批量上传

## 常见问题

**Q：删除后还能在分组里看到文档吗？**  
A：可以保留分组归属，但不会参与问答。可在垃圾箱恢复。

**Q：永久删除失败怎么办？**  
A：查看事件时间线与技术详情中的 warning；文档可能仍为 TRASHED，可重试 purge。

**Q：Ask 搜不到内容，文档是否在垃圾箱？**  
A：检查文档管理与垃圾箱；恢复后即可重新检索。

## 页面入口

- 垃圾箱：`/trash.html`
- 系统设置 → 存储空间 / 缓存管理：`/settings.html`
