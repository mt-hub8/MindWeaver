# RAG Hybrid Retrieval Fusion (V3.0)

## 目标

V3.0 在应用层引入 **Hybrid Retrieval（混合检索）**：在保留现有 dense vector retrieval 的同时，增加低依赖 lexical keyword retrieval，并通过 **RRF（Reciprocal Rank Fusion，倒数排名融合）** 合并两路候选，可选复用 V2.9 reranker，并支持通过 V2.8/V2.9 评估框架对比 dense-only 与 hybrid 的质量差异。

本版本为 **App-layer Hybrid**，不依赖 Qdrant sparse vector、Elasticsearch、BM25 索引或外部搜索引擎。

## Dense vs Lexical

| 路径 | 含义 | 本版本实现 |
|------|------|------------|
| **Dense** | 基于 query embedding 的向量相似度检索 | 复用现有 `VectorStore`（ExactCosine / Qdrant） |
| **Lexical** | 基于关键词 / token 重叠的检索 | `SimpleLexicalRetriever`（token overlap + 简单 TF） |

Dense 擅长语义相近但字面不同的匹配；Lexical 擅长精确关键词命中。Hybrid 通过 RRF 合并两路排名，使双路命中的 chunk 获得更高融合分。

## App-layer Hybrid 流程

**Hybrid 关闭（默认）**：与 V2.9 一致。

- 无 rerank：`query embedding → VectorStore topK → citations`
- 有 rerank：`VectorStore candidateTopK → rerank → finalTopK`

**Hybrid 开启**：

1. Dense retrieval：`denseTopK` 条向量候选
2. Lexical retrieval：`lexicalTopK` 条关键词候选（来自 DB `document_chunk`）
3. **RRF fusion**：按 chunk 去重合并，双路命中时 fusion score 叠加
4. （可选）Rerank：对 fused 候选 rerank
5. 取 `finalTopK`（来自 API 请求 `topK`）构造 citations 与 RAG prompt

```
query
  ├─ dense (VectorStore, denseTopK)
  ├─ lexical (SimpleLexicalRetriever, lexicalTopK)
  └─ RRF fusion → [optional rerank] → finalTopK → citations → LLM
```

Query embedding **不写入** `embedding_cache`（与 V2.7 边界一致）。

## SimpleLexicalRetriever

- **数据源**：`DocumentChunkRepository`（`findAll()` 或按 `documentId` 过滤）
- **分词**：小写化，按非字母数字切分，忽略空 token
- **打分**（非 BM25，可解释、可测试）：

```
overlapRatio = matchedQueryTokens / queryTokenCount
tfBonus      = sum(min(tf(token), 3) for matched tokens) / (queryTokenCount * 3)
lexicalScore = 0.8 * overlapRatio + 0.2 * tfBonus
```

- 按 `lexicalScore` 降序，取 `lexicalTopK`
- **局限**：全表扫描 chunk，无倒排索引；不等价于 BM25；语料很大时延迟会上升

## RRF（Reciprocal Rank Fusion）

对每个候选列表（dense / lexical），chunk 贡献：

```
contribution = 1 / (k + rank)
```

- `rank`：该路结果中的 **1-based** 排名
- `k`：平滑常数，默认 **60**（Cormack et al. 常用值，配置项 `rag.hybrid.rrf-k`）

同一 `chunkId` 在 dense 与 lexical 均出现时，两路 contribution **相加** 为 `fusionScore`。按 `fusionScore` 降序得到 fused 列表。

## TopK 含义

| 配置项 | 含义 |
|--------|------|
| `denseTopK` | 向量检索召回候选数（hybrid 开启时） |
| `lexicalTopK` | 关键词检索召回候选数 |
| `finalTopK` | 融合（及可选 rerank）后返回给 RAG 的条数；API 请求 `topK` 优先 |
| `rag.hybrid.final-top-k` | 配置默认值（5），实际以请求 `topK` 为准 |

约束：`denseTopK >= finalTopK`；`lexicalTopK >= 1`。

## Hybrid 与 Rerank 同时启用

顺序固定为：

**dense → lexical → RRF fusion → rerank → finalTopK**

Rerank 的输入为 fused 候选（`originalScore` 使用 `fusionScore`）。Citation 中同时可包含 hybrid 字段（dense/lexical rank、fusionScore）与 rerank 字段（originalRank、rerankScore）。

## 配置

`application.properties`（默认均关闭 hybrid）：

```properties
rag.hybrid.enabled=false
rag.hybrid.dense-top-k=20
rag.hybrid.lexical-top-k=20
rag.hybrid.final-top-k=5
rag.hybrid.fusion=rrf
rag.hybrid.rrf-k=60
```

启用 hybrid：设置 `rag.hybrid.enabled=true`，按需调整 topK 与 `rrf-k`。非法 fusion 策略（非 `rrf`）会在检索时抛出校验错误。

## Retrieval Metadata（`POST /rag/answers` 响应）

在 V2.9 rerank 字段基础上 **增量** 增加（`hybridEnabled=false` 时为 null / 不出现）：

- `hybridEnabled`
- `denseTopK`, `lexicalTopK`, `fusionStrategy`
- `denseCandidateCount`, `lexicalCandidateCount`, `fusedCandidateCount`
- `hybridLatencyMs`

Citation 可选字段：`denseRank`, `lexicalRank`, `denseScore`, `lexicalScore`, `fusionScore`, `denseHit`, `lexicalHit`。

## Evaluation：Dense vs Hybrid

评估为 **opt-in**，默认测试不运行。

```properties
app.evaluation.retrieval.enabled=true
app.evaluation.retrieval.compare-hybrid=true
app.evaluation.retrieval.dense-top-k=20
app.evaluation.retrieval.lexical-top-k=20
app.evaluation.retrieval.hybrid-rrf-k=60
# 可选：两路都走 rerank
app.evaluation.retrieval.compare-hybrid-rerank=true
```

启动应用后自动读取 `docs/evaluation/rag-retrieval-eval-cases.json`，输出：

| 文件 | 路径 |
|------|------|
| JSON | `docs/evaluation/reports/rag-hybrid-comparison-report.json` |
| Markdown | `docs/evaluation/reports/rag-hybrid-comparison-report.md` |

报告包含：**dense baseline summary**、**hybrid summary**、**delta**、per-case **IMPROVED / REGRESSED / UNCHANGED**、missed cases。不要求 hybrid 必须提升指标。

Runner 优先级：`compare-hybrid` > `compare-rerank` > 单次评估。

## 为何不做 Qdrant sparse / ES / BM25

- 避免改动 Qdrant collection schema 与运维复杂度
- 避免引入 Elasticsearch/OpenSearch 与额外基础设施
- 本版本聚焦 **可替换抽象 + 低依赖默认测试**；生产级 lexical 可后续替换 `LexicalRetriever` 实现

## 为何默认测试不依赖 Qdrant / Worker / 外部模型

- `SimpleLexicalRetriever`、`RrfFusionRanker` 使用单元测试与 mock
- `RagTwoStageRetrievalService` hybrid 路径使用 mock `LexicalRetriever` / `FusionRanker`
- 评估 hybrid 对比测试 mock `DocumentEmbeddingService` 与 `LexicalRetriever`
- 真实 Qdrant + worker 评估见 `docs/manual/rag-retrieval-evaluation.md`（manual / opt-in）

## 常见失败排查

| 现象 | 可能原因 |
|------|----------|
| hybrid 开启后 lexical 候选为 0 | DB 无 chunk 或 query token 与 chunk 无重叠 |
| `denseTopK must be >= finalTopK` | 增大 `rag.hybrid.dense-top-k` 或减小请求 `topK` |
| `Unsupported fusion strategy` | 仅支持 `rrf`，检查 `rag.hybrid.fusion` |
| hybrid 未提升指标 | 预期行为；查看 comparison 报告 delta 与 per-case outcome |
| 评估未生成 hybrid 报告 | 确认 `app.evaluation.retrieval.enabled=true` 且 `compare-hybrid=true` |

## 相关文档

- [RAG Retrieval Evaluation](rag-retrieval-evaluation.md)
- [Two-Stage Retrieval & Rerank](rag-two-stage-retrieval-rerank.md)
