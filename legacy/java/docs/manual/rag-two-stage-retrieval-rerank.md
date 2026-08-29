# RAG Two-Stage Retrieval & Rerank Manual（V2.9）

本文说明 V2.9 的两阶段检索（two-stage retrieval）与 rerank 能力。

## 1. V2.9 目标

在现有 RAG 检索链路上新增：

```text
query embedding
  -> VectorStore search candidateTopK
  -> Reranker rerank
  -> finalTopK
  -> citations + RAG prompt
```

本版本只优化 retrieval ranking，不评估 LLM answer quality。

## 2. Two-Stage Retrieval 流程

| 阶段 | 说明 |
| --- | --- |
| Stage 1 | VectorStore 扩大候选召回 `candidateTopK` |
| Stage 2 | Reranker 对候选 chunk 重排序 |
| Final | 取 `finalTopK` 进入 citations 与 RAG prompt |

默认关闭 rerank 时，行为与 V2.7 一致：直接 VectorStore `topK` 检索。

## 3. candidateTopK / finalTopK

- `candidateTopK`：第一阶段向量检索召回数量（默认 20）
- `finalTopK`：rerank 后保留数量（默认 5；RAG API 请求 `topK` 作为 finalTopK）
- 约束：`candidateTopK >= finalTopK`，否则返回校验错误

## 4. Reranker 抽象

接口：`com.tuoman.ai_task_orchestrator.rerank.Reranker`

输入：

- query
- candidate results（含 originalRank / score / content）
- finalTopK

输出：

- reranked results
- rerankerName
- latencyMs

## 5. 默认 LexicalOverlapReranker

Provider：`lexical`

Scoring 规则（简单可解释）：

```text
lexicalScore = matched query tokens / total query tokens
combinedScore = 0.7 * lexicalScore + 0.3 * originalVectorScore
```

局限：

- 仅基于词面重合，不理解语义；
- 不保证所有场景指标提升；
- 不是 CrossEncoder / 外部 rerank API。

## 6. 配置项

`application.properties` 默认：

```properties
rag.rerank.enabled=false
rag.rerank.candidate-top-k=20
rag.rerank.final-top-k=5
rag.rerank.provider=lexical
```

启用示例：

```properties
rag.rerank.enabled=true
rag.rerank.candidate-top-k=20
rag.rerank.provider=lexical
```

RAG Answer API 请求字段不变，仍使用 `query` + `topK`。

## 7. RAG Answer API 行为

### rerank disabled（默认）

- VectorStore 直接 search `topK`
- citations 顺序保持原始检索顺序
- retrieval metadata 中 `rerankEnabled=false`

### rerank enabled

- VectorStore search `candidateTopK`
- Reranker 重排后取 `finalTopK`（来自请求 `topK`）
- citations 顺序来自 rerank 后顺序
- retrieval metadata 增加 rerank 字段

## 8. Retrieval Metadata 新增字段

- `rerankEnabled`
- `rerankerName`
- `candidateTopK`
- `finalTopK`
- `rerankLatencyMs`

citations 可选字段（rerank enabled 时出现）：

- `originalRank`
- `rerankedRank`
- `originalScore`
- `rerankScore`

## 9. Baseline vs Rerank Evaluation

复用 V2.8 evaluation framework，新增 compare 模式。

配置：

```properties
app.evaluation.retrieval.enabled=true
app.evaluation.retrieval.compare-rerank=true
app.evaluation.retrieval.candidate-top-k=20
app.evaluation.retrieval.document-id=1
```

启动示例：

```powershell
cd E:\code\ai-task-orchestrator
.\mvnw.cmd spring-boot:run "-Dspring-boot.run.profiles=docker,qdrant" "-Dspring-boot.run.arguments=--app.embedding.provider=local-worker --app.evaluation.retrieval.enabled=true --app.evaluation.retrieval.compare-rerank=true --app.evaluation.retrieval.document-id=1"
```

报告输出：

- JSON：`docs/evaluation/reports/rag-retrieval-comparison-report.json`
- Markdown：`docs/evaluation/reports/rag-retrieval-comparison-report.md`

报告包含 baseline metrics、rerank metrics、delta、improved/regressed/missed cases。

## 10. 为什么默认测试不跑真实 evaluation

`.\mvnw.cmd test` 必须保持低依赖边界，不连接 Qdrant/Docker/worker/外部网络。  
真实 baseline vs rerank 评估属于 manual opt-in。

## 11. 常见失败排查

- rerank 未生效：确认 `rag.rerank.enabled=true`
- candidateTopK 配置非法：确保 `candidateTopK >= finalTopK`
- rerank 后仍 miss：lexical rerank 不一定提升，查看 comparison report 的 regressed cases
- evaluation 对比无数据：确认 demo 文档已 ingest + embed，且 `document-id` 正确
- metadata 无 rerank 字段：默认 rerank disabled 时字段为 null / false
