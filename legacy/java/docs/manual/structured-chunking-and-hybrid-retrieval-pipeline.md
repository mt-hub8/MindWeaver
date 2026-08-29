# V15.0 结构化切分与混合检索主链路优化

## 目标

将 V14 知识库体检的诊断建议落到工程实现：结构化 chunk、metadata 增强、预过滤、BM25/keyword 检索、Hybrid + RRF、Reranker 抽象、parent/adjacent 上下文回填、重新索引与检索诊断。

## 为什么万级文档不能只用向量检索

专有名词、接口名、版本号、配置项在纯向量空间容易与语义相近但错误的 chunk 混淆。Hybrid 检索 + metadata pre-filter 可降低跨 collection / 错误版本污染。

## 结构化 Chunk Schema

`document_chunk` 新增：`chunk_uid`、`parent_chunk_id`、`previous_chunk_id`、`next_chunk_id`、`section_path`、`chunk_type`、`content_hash`、`doc_type`、`collection_id`、`version`、`metadata_status` 等。缺失字段允许为 null，不阻断导入。

## section_path

Markdown 标题层级切分后写入 `section_path`（与 `heading_path` 兼容），例如 `Root > Child`。

## Metadata Pre-filter

`RetrievalFilter` 支持 collectionId、version、docType、status、includeDeprecated/includeDraft/includeTrashed。默认 TRASHED/PURGED 永远过滤，DEPRECATED 默认过滤。

## BM25 / Keyword Retrieval

`KeywordRetriever` 接口 + `SimpleKeywordRetriever`（非 Elasticsearch）。标题 / section_path 加权更高。

## Hybrid + RRF

`HybridRetrievalService` 合并 vector 与 keyword 候选，经 `RrfFusionService`（k=60）融合。

## Reranker

`NoopReranker`、`SimpleHeuristicReranker`、`LexicalOverlapReranker`（既有）。通过 `rag.rerank.provider` 或 `app.retrieval.reranker-mode` 选择。

## Parent / Adjacent Context

`ContextExpansionService` 支持 ADJACENT / PARENT / PARENT_AND_ADJACENT，受 `maxContextChars` 限制。

## Reindex

`POST /documents/{id}/reindex`（既有）、`POST /retrieval/collections/{id}/reindex`、`POST /retrieval/reindex`。记录 `retrieval_reindex_event`。

## 与 V14 关系

V14 评测 run 仍可使用 `HYBRID_RRF` 等策略；V15 优化后 CrossCollectionLeakRate / WrongVersionLeakRate 应随 metadata filter 降低。

## 边界

- 不做 OCR / Elasticsearch / 真实 cross-encoder
- BM25 为 simple keyword 实现
- filter 默认 APPLICATION_SIDE

## FAQ

**如何启用混合检索？** 设置 `rag.hybrid.enabled=true` 或 `app.retrieval.hybrid-enabled=true`（默认 false，不破坏 mock 测试）。

**如何查看当前配置？** 访问 `/retrieval-settings.html` 或 `GET /retrieval/settings`。
