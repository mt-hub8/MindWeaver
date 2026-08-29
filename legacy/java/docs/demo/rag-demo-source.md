# AI Task Orchestrator RAG Demo Source

本文件专为 RAG Demo 手工验证设计。通过 `POST /documents` 上传后，系统会按 Markdown 标题自动切分 chunk，再经 `POST /documents/{documentId}/embeddings` 写入 VectorStore，供 `POST /rag/answers` 与 `/rag-demo.html` 检索问答。

---

# AI Task Orchestrator 是什么

[EVIDENCE:project-overview]
AI Task Orchestrator 是一个面向学习与工程演示的后端系统。它的目标是把异步任务编排、可靠消息投递、LLM 调用抽象、文档上传与切分、Embedding、向量检索、检索评估以及 RAG Answer API 串成一条可手工验证的闭环。项目强调可观测的 metadata、可复现的手工路径，以及在不引入重型前端框架的前提下完成端到端演示。

---

# RAG Answer API 的核心流程

[EVIDENCE:rag-answer-pipeline]
RAG Answer API 的入口是 `POST /rag/answers`。核心流程为：校验 query 与 topK；使用当前 active EmbeddingProvider 为 query 生成 embedding（query embedding 不经过 Embedding Cache）；调用 VectorStore 做 TopK 向量检索；将命中的 chunk 构造成 citations；若未检索到任何 chunk，则返回 no-context 固定答案且跳过 LLM；若有检索结果，则由 RagPromptBuilder 组装上下文 prompt，再调用 LlmClient（默认 Mock LLM）生成 answer。响应包含 answer、citations、retrieval metadata 与 generation metadata。

---

# Citations 的作用

[EVIDENCE:rag-citation-role]
Citations 用于把生成答案与检索到的来源 chunk 关联起来。每条 citation 至少包含 sourceIndex、documentId、chunkId、score 与 contentSnippet，便于 reviewer 判断答案是否基于文档内容。Citations 解决的是可追溯性问题：用户不仅看到 answer，还能看到系统引用了哪些 chunk。当前版本是 chunk-level citation，不是 sentence-level 精确到句子的引用，也不提供 citation 跳转或 chunk 详情页。

---

# Embedding Cache 的设计与 Cache Key

[EVIDENCE:embedding-cache-key]
Embedding Cache 用于在文档入库（ingestion）阶段避免对相同 chunk 重复调用 Embedding Provider。Cache key 由四个字段共同组成：chunkHash、provider、model、dimension。不能只用 chunkHash，因为同一段文本在不同 provider 或不同 model 下属于不同向量空间，混用会导致检索错误。RAG Answer API 的 query embedding 当前不经过 Embedding Cache，Cache 主要服务 `POST /documents/{documentId}/embeddings` 链路。可通过 `GET /embedding-cache/metrics` 观察 hit、miss、write、conflict 等指标。

---

# Minimal Web UI 为什么不引入 React 或 Vue

[EVIDENCE:minimal-web-ui-rationale]
V2.7.1 的 Minimal Web UI 通过 Spring Boot static resources 提供 `/rag-demo.html`，使用原生 HTML、CSS 与 JavaScript 调用 `POST /rag/answers`。不引入 React、Vue、npm、TypeScript 或前端构建工具，是为了保持演示入口极简、零构建、与后端同源部署，避免为求职项目增加不必要的前端工程复杂度。页面职责仅限于：输入 query 与 topK、展示 loading 与 error、渲染 answer、citations 与 JSON metadata，不做多轮对话、streaming 或文档管理。

---

# Qdrant Benchmark 与 Manual Verification 的意义

[EVIDENCE:qdrant-benchmark-purpose]
Qdrant Manual Verification 用于在真实 Qdrant 容器下验证文档上传、embedding、upsert 与 search 的端到端链路，通常配合 `docker,qdrant` Spring profile。Qdrant Benchmark（V2.6.7）则在固定 retrieval dataset 下对比 ExactCosineVectorStore 与 QdrantVectorStore 的检索指标与延迟，并将结果写入 `build/reports/retrieval/`。Benchmark 是手工 opt-in 流程，不是默认 CI 门禁，也不能直接推导“Qdrant 一定更快”。其意义在于：在相同 corpus、query cases、topK 与 embedding 空间下，采集可复现的对比证据，而不是凭感觉选型。
