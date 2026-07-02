# Upload-to-Ask 产品闭环（V3.2）

## 流程

1. 打开 **Documents**（`/documents.html`）
2. 选择并上传 `.txt`、`.md` 或文本型 `.pdf`
3. 等待同步 ingestion 完成（页面显示 summary：`documentId`、`chunkCount`、`embeddingCount`、`vectorWriteCount`）
4. 打开 **Ask**（`/ask.html`）提问
5. 查看 **Answer**、**Citations** 与 retrieval metadata

## API

```http
POST /documents/upload
Content-Type: multipart/form-data

file=<document>
```

成功响应字段：`documentId`、`title`、`status`（`READY`）、`chunkCount`、`embeddingCount`、`vectorWriteCount`。

兼容旧接口：`POST /documents`（仅 txt/md，仅切块）、`POST /documents/{id}/embeddings`（单独 embedding）。

## PDF 支持边界

| 支持 | 不支持 |
|------|--------|
| 文本型 PDF（可选中复制文字） | 扫描版 PDF / OCR |
| Apache PDFBox 文本提取 | 图片识别 |
| 最大 2MB（可配置） | 加密 PDF |
| | Word / Excel / URL 抓取 |
| | 复杂表格结构还原 |

扫描版或无可提取文本的 PDF 将返回：

`PDF has no extractable text. Scanned PDFs/OCR are not supported in this version.`

## 配置

```properties
app.document.ingestion.max-file-size-bytes=2097152
spring.servlet.multipart.max-file-size=2MB
```

## 相关页面

- [RAG Hybrid Retrieval Fusion](rag-hybrid-retrieval-fusion.md)
- [RAG Demo Golden Path](rag-demo-golden-path.md)
