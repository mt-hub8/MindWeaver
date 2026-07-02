# Upload-to-Ask 产品闭环（V3.3 异步摄入）

## 流程

1. 打开 **文档管理**（`/documents.html`）
2. 选择并上传 `.txt`、`.md` 或文本型 `.pdf`
3. 接口快速返回 `taskId`（HTTP 202），页面展示「待处理」
4. 等待后台异步完成切块、向量生成、知识库索引写入，状态变为「已完成」
5. 打开 **知识库问答**（`/ask.html`）提问
6. 查看 **回答**、**检索引用（Citation）** 与 retrieval metadata

## API

### 异步上传

```http
POST /documents/upload
Content-Type: multipart/form-data

file=<document>
```

成功响应（HTTP 202）字段：`taskId`、`documentId`、`filename`、`status`、`displayStatus`、`displayMessage`。

### 摄入任务查询

```http
GET /documents/ingestions/{taskId}
GET /documents/ingestions
POST /documents/ingestions/{taskId}/retry
```

`retry` 仅支持 `FAILED` 状态，默认最多重试 3 次。

兼容旧接口：`POST /documents`（仅 txt/md，仅切块）、`POST /documents/{id}/embeddings`（单独 embedding）。

## PDF 支持边界

| 支持 | 不支持 |
|------|--------|
| 文本型 PDF（可选中复制文字） | 扫描版 PDF / OCR |
| Apache PDFBox 文本提取 | 图片识别 |
| 最大 2MB（可配置） | 加密 PDF |
| | Word / Excel / URL 抓取 |

扫描版或无可提取文本的 PDF 将返回中文友好错误信息。

## 配置

```properties
app.document.ingestion.max-file-size-bytes=2097152
app.document.ingestion.max-retry-count=3
app.document.ingestion.recent-task-limit=20
spring.servlet.multipart.max-file-size=2MB
```

## 相关页面

- [RAG Hybrid Retrieval Fusion](rag-hybrid-retrieval-fusion.md)
- [RAG Demo Golden Path](rag-demo-golden-path.md)
