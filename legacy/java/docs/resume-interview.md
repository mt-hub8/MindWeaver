# 简历与面试表达文档

## 一、文档目的

本文档整理 AI Task Orchestrator 当前阶段的简历表达、面试表达、技术亮点、诚实边界，以及 benchmark 数据存在 / 不存在时的写法模板。

---

## 二、项目一句话描述

AI Task Orchestrator 是一个基于 Spring Boot、RabbitMQ、MySQL 和 Flyway 构建的异步 AI 任务编排与 RAG 检索后端系统。当前已实现可靠任务执行底座（Transactional Outbox、Atomic Claim、Task Attempt）、文档上传与 chunking、EmbeddingProvider 抽象、VectorStore 抽象、检索评估与基础 RAG 问答链路。

---

## 三、简历版本描述

**一句话版本**

基于 Spring Boot、RabbitMQ、MySQL 实现异步 AI 任务编排与 RAG 检索后端，支持 Transactional Outbox、Atomic Claim、EmbeddingProvider / VectorStore 抽象、Retrieval Evaluation 与 Mock LLM RAG 问答。

**三行版本**

AI Task Orchestrator 模拟 AI Agent / LLM 平台的长耗时任务执行与 RAG 检索场景。

项目使用 RabbitMQ + Transactional Outbox 解耦任务创建与执行，用状态机、Task Attempt、重试、幂等、取消和超时保证任务生命周期可靠。

当前已扩展 Document Upload / Chunking、EmbeddingProvider（mock / OpenAI-compatible / local-worker）、VectorStore（ExactCosine / Qdrant）、Retrieval Evaluation Harness 与 RAG Answer API（Mock LLM）。

**完整项目经历版本**

- 项目背景：真实 AI 任务具有长耗时、易失败、需要重试与状态追踪等特点；RAG 检索需要 chunking、embedding、向量存储与评估体系。
- 技术栈：Java 21、Spring Boot、Spring Data JPA、MySQL、Flyway、RabbitMQ、Docker Compose、Python FastAPI（local worker 原型）。
- 本人工作：
  - 设计任务状态机、Transactional Outbox、Atomic Claim 与 Task Attempt。
  - 实现 RabbitMQ 异步调度、失败重试、幂等消费、取消与超时。
  - 设计 LLM Client 抽象、Prompt Template、Model Router。
  - 实现 Document Upload、Fixed / Adaptive Chunking。
  - 设计 EmbeddingProvider 抽象，接入 Mock / OpenAI-compatible / Local Worker。
  - 设计 VectorStore 抽象，实现 ExactCosineVectorStore 与 QdrantVectorStore 原型。
  - 实现 Retrieval Evaluation Harness（Recall@K、MRR、NDCG@K 等）与 Benchmark Harness。
  - 打通 RAG Answer with Citation 基础链路（Mock LLM）。
- 核心成果：完成可本地运行、可验收、可扩展的 AI 任务编排与 RAG 检索底座。
- 工程亮点：Outbox 一致性、Atomic Claim、provider 抽象、评估 harness、测试与外部依赖隔离。

---

## 四、简历 bullet 示例

**Reliable Async Task Execution**

- 基于 Spring Boot 和 RabbitMQ 实现异步任务调度，将任务创建与后台执行解耦。
- 设计 Transactional Outbox，保证任务入库与 MQ 投递的一致性。
- 实现 Atomic Claim 与 Task Attempt，记录每次 LLM 执行明细。
- 设计任务状态机和 `task_event`，防止非法状态流转。
- 实现失败处理、自动重试、Consumer 入口幂等、协作式取消和超时扫描。

**LLM / Prompt**

- 设计 `LlmClient` 抽象与 `MockLlmClient`，解耦任务执行与模型调用。
- 设计 Prompt Template 数据模型与渲染器，保存 `renderedPrompt`。
- 设计 `ModelRouter`，支持 requested model 与实际执行 model 分离。
- 记录 LLM provider、token usage 和 latency。

**Document / RAG**

- 实现 `.txt/.md` 文档上传与 Fixed / Adaptive Chunking。
- 设计 `EmbeddingProvider` 抽象，支持 Mock / OpenAI-compatible / Local Worker。
- 设计 `VectorStore` 抽象，实现 `ExactCosineVectorStore` 与 `QdrantVectorStore` 原型。
- 实现 Document Search TopK 与 Retrieval Evaluation Harness（Recall@K、MRR、NDCG@K 等）。
- 实现 RAG Answer with Citation API（当前 Mock LLM）。
- 编写 VectorStore / Embedding Provider Benchmark Harness，用于对比 baseline 与 candidate。

**工程化**

- 使用 Flyway 管理数据库结构演进。
- 使用 Docker Compose 搭建本地 MySQL / RabbitMQ 环境。
- 默认 `mvn test` 不依赖 Qdrant、Python worker、OpenAI API 或外部网络。

---

## 五、没有真实线上数据时怎么写

**原则**

- 不写“提升 X% Recall@K”或“NDCG 提高 Y%”——没有 benchmark 跑分数据就不要写数字。
- 不写“Qdrant 生产可用”或“Local Worker 已上线”——它们是实验性原型。
- 不写“已实现 Rerank / Hybrid Search / Agent Runtime”——这些尚未实现。
- 强调**架构设计、抽象边界、评估 harness、可扩展性**，而不是虚构业务指标。

**推荐表述**

- “设计 EmbeddingProvider / VectorStore 抽象，默认测试使用 mock / exact baseline，Qdrant 与 local-worker 为手工验证路径。”
- “实现 Retrieval Evaluation Harness，支持 Recall@K、MRR、NDCG@K 等指标计算；benchmark seed 位于测试资源目录。”
- “实现 VectorStore Benchmark Harness，在测试中对比 baseline 与 candidate 的检索指标与延迟，未暴露为生产 HTTP API。”
- “RAG Answer API 已打通检索 + citation + Mock LLM 生成链路，LLM 层仍为 Mock，未做 Generation Evaluation。”

**避免表述**

- ❌ “Qdrant 将 Recall@5 提升了 20%。”
- ❌ “Local Embedding Worker 已在生产环境部署。”
- ❌ “系统已支持 Hybrid Search 与 Rerank。”
- ❌ “RAG 回答质量达到生产标准。”

---

## 六、有 benchmark 数据后怎么写（模板，不填不存在的数据）

仅在**真实跑过 benchmark 并保存结果**后，才使用以下模板，并将 `{...}` 替换为实际数据：

```
在 retrieval benchmark dataset（{dataset_name}，{case_count} cases）上，
ExactCosineVectorStore baseline 的 Recall@{K} = {baseline_recall}；
切换为 {candidate_name} 后 Recall@{K} = {candidate_recall}（Δ = {delta}）。
P95 search latency：baseline {baseline_latency_ms}ms vs candidate {candidate_latency_ms}ms。
```

```
Embedding Provider 对比（{provider_a} vs {provider_b}）：
MRR = {mrr_a} vs {mrr_b}，NDCG@5 = {ndcg_a} vs {ndcg_b}。
```

**注意**

- 数据来源必须可复现（测试命令、配置、dataset 版本）。
- 不要跨 embedding space 或不同 chunk 策略直接对比而不说明前提。
- Qdrant 对比需注明是否真实 Qdrant 实例、collection 配置、embedding provider。
- Local Worker 对比需注明模型名称、首次加载耗时、是否 warm-up。

---

## 七、面试开场怎么讲

这个项目模拟 AI Agent / LLM 平台里的长耗时任务执行，并在同一底座上扩展 RAG 检索能力。

任务侧：Spring Boot 提供 API，RabbitMQ 解耦创建与执行，Transactional Outbox 保证一致性，Atomic Claim 与 Task Attempt 记录每次执行，状态机 + 重试 + 超时保证生命周期可靠。

RAG 侧：支持文档上传与 chunking，EmbeddingProvider 抽象（默认 mock），VectorStore 抽象（默认 exact cosine），Document Search 与 Retrieval Evaluation API 已可用。Qdrant 与 Local Worker 是实验性手工验证路径。

RAG Answer API 已打通检索 + citation + Mock LLM，但生成质量未做 Generation Evaluation，也不应声称 production-ready。

---

## 八、Reliable Async Task Execution 怎么讲

```text
POST /tasks
-> TaskService 同事务写 task + task_event + task_outbox
-> Outbox Dispatcher -> RabbitMQ
-> Consumer -> Atomic Claim -> TaskAttempt
-> PromptTemplate / ModelRouter / MockLlmClient
-> output chunks + 状态更新
-> RetryScheduler / TimeoutScheduler
```

**Transactional Outbox**：避免“DB 已提交但 MQ 未发送”。

**Atomic Claim**：只有 `PENDING / RETRY_PENDING` 可进入 `RUNNING`，重复消息被忽略。

**Task Attempt**：每次执行单独记录 provider、model、token、latency，便于审计与重试分析。

---

## 九、EmbeddingProvider 怎么讲

`EmbeddingProvider` 是统一向量化接口。当前有三种实现：

- `MockEmbeddingProvider`（默认）：确定性 mock，测试不依赖外部服务。
- `OpenAiCompatibleEmbeddingProvider`：需 API key，手工配置。
- `LocalEmbeddingWorkerEmbeddingProvider`：调用 Python FastAPI worker，需手工启动。

`DocumentEmbeddingService` 负责 embed 文档 chunks，写入 `document_chunk_embedding` 并 upsert 到 VectorStore。search 时使用同一 provider/model 空间，避免混用不同 embedding space。

**边界**：Local Worker 是原型，不是 production deployment；默认 CI 不启动 Python worker。

---

## 十、VectorStore 怎么讲

`VectorStore` 抽象 upsert / search / delete。两种实现：

- `ExactCosineVectorStore`（默认）：基于 DB 中 embedding 做 exact cosine，适合 baseline 与本地开发。
- `QdrantVectorStore`（实验性）：REST 接入 Qdrant，需手工启动 Qdrant 并配置 `app.vector-store.provider=qdrant`。

`DocumentEmbeddingService.search()` 委托 VectorStore，不再内联 cosine 计算。

**VectorStore Benchmark Harness**（测试 only）：`VectorStoreBenchmarkRunner` 对比 baseline vs candidate 的 Recall@K、延迟等，通过 `VectorStoreBenchmarkComparisonTest` 验证。

**边界**：不要声称 Qdrant 已 production-ready；不要声称 Qdrant 提升了 Recall@K（除非有真实 benchmark 数据）。

---

## 十一、Retrieval Evaluation 怎么讲

`POST /evaluations/retrieval` 接受 documentId、topKValues、cases（含 expectedChunkIds）。

`RetrievalMetricsCalculator` 计算 Recall@K、Precision@K、HitRate@K、MRR、NDCG@K、ContextPrecision@K。

Benchmark seed（`retrieval-corpus-v1.md` + `retrieval-benchmark-v1.json`）通过 `BenchmarkEvidenceMapper` 将 evidence marker 映射为 chunkId，主要用于测试 harness。

**边界**：Evaluation result 未持久化；Generation Evaluation（Faithfulness 等）尚未实现。

---

## 十二、RAG Answer 怎么讲

`POST /rag/answer`：检索 TopK chunks → `RagPromptBuilder` 构造 prompt → Mock LLM 生成 answer → 返回 citations。

链路已打通，但：

- LLM 是 Mock，不是真实 OpenAI / Claude。
- 未做 Generation Evaluation 或 citation faithfulness 验证。
- 不应表述为“完整 production RAG”。

---

## 十三、Local Embedding Worker 怎么讲

Python FastAPI + sentence-transformers，位于 `workers/embedding-worker/`。

Java 侧 `LocalEmbeddingWorkerEmbeddingProvider` 通过 HTTP 调用 `POST /embeddings`。

**诚实边界**：

- 需手工启动，未纳入 docker-compose。
- 首次模型加载耗时不定。
- 默认 `mvn test` 不依赖 worker。
- 不要声称“生产可用”或“已大规模部署”。

---

## 十四、当前项目边界

**已实现（可诚实表述）**

- Transactional Outbox、Atomic Claim、Task Attempt
- Mock LLM、Prompt Template、Model Router、output chunks
- Document Upload、Fixed / Adaptive Chunking
- EmbeddingProvider 抽象（mock / openai / local-worker）
- VectorStore 抽象（exact / qdrant）
- Document Search、Retrieval Evaluation、RAG Answer（Mock LLM）
- Benchmark harness（测试验证）

**原型 / 实验性（需加限定词）**

- Local Embedding Worker、QdrantVectorStore、OpenAI embedding 手工配置
- RAG Answer（Mock LLM）

**尚未实现（不要声称已有）**

- Rerank、Hybrid Search / BM25
- Production-grade RAG generation / Generation Evaluation
- Auth / tenant / quota、API rate limit
- Agent Runtime、KV Cache-aware scheduling
- Production observability dashboard
- Distributed worker registry
- Real billing / subscription

---

## 十五、如何诚实回答“这是 AI 项目吗？”

这是一个 **AI 任务编排与 RAG 检索后端原型**，不是完整 Agent 平台。

它先解决可靠异步任务执行（Outbox、Claim、Attempt），再在同一底座上扩展 chunking、embedding、vector search、retrieval evaluation 和基础 RAG 问答。

LLM 层当前是 Mock；Embedding 默认是 Mock；VectorStore 默认是 exact cosine。OpenAI、Local Worker、Qdrant 是可选实验路径。

价值在于：架构边界清晰、provider 可替换、评估 harness 可复现、测试与外部依赖隔离——为后续接入真实模型、Rerank、Hybrid Search、Agent Runtime 打基础。

---

## 十六、后续扩展路线（面试可提，非已实现）

1. E2E Demo Golden Path
2. Atomic Finalization
3. Qdrant Manual Verification（文档化 + 可选 compose）
4. API Error Response Standardization
5. Local Embedding Worker Packaging
6. Retrieval Policy & VIP Search
7. Rerank
8. Hybrid Search
9. RAG Answer with Citation（真实 LLM + Generation Evaluation）
10. Generation Evaluation
11. Agent Runtime
12. KV Cache-aware Scheduling

不要把已完成的 VectorStore、QdrantVectorStore、Retrieval Evaluation 继续说成“下一步要做”。

---

## 十七、面试常见问题

1. **为什么用 Transactional Outbox？**  
   保证任务入库与 MQ 投递原子性，避免消息丢失或重复发送不一致。

2. **Atomic Claim 和幂等有什么关系？**  
   Claim 只允许合法状态进入 RUNNING，重复 MQ 消息不会重复执行。

3. **EmbeddingProvider 和 VectorStore 为什么分开？**  
   Embedding 是“文本 → 向量”，VectorStore 是“向量存储与检索”，解耦后可独立替换 provider 或存储后端。

4. **ExactCosine 和 Qdrant 怎么选？**  
   Exact 适合 baseline 与小规模本地验证；Qdrant 适合实验性 ANN 检索，需手工部署，默认测试用 fake client。

5. **Retrieval Evaluation 的 expectedChunkIds 从哪来？**  
   手工指定或 benchmark seed + Evidence Mapper 映射；mapper 主要用于测试 harness。

6. **RAG Answer 算完成了吗？**  
   链路打通，但 LLM 是 Mock，未做 generation quality 评估，不是 production RAG。

7. **如果要生产化还缺什么？**  
   Auth、多租户、监控、Rerank、Hybrid Search、真实 LLM governance、Worker 编排、Qdrant 运维等。

---

## 十八、相关文档

- [README.md](../README.md)
- [docs/local-dev.md](local-dev.md)
- [docs/project-structure.md](project-structure.md)
- [docs/api-examples.md](api-examples.md)
- [docs/interview/](../interview/)（版本演进详细文档）
