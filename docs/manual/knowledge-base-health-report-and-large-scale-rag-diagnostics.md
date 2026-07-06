# V14.0 知识库体检报告与万级文档 RAG 诊断

## 目标

V14.0 将 MindWeaver 从「能问答的知识库」升级为**知识库质量诊断系统**。用户上传大量中文文档后，系统不仅回答问题，还能评估：

- 检索质量（召回、排序、上下文精确率）
- 隔离质量（跨集合污染、错误版本污染）
- 引用质量与生成忠实性（启发式，非 LLM judge）

并输出 **0–100 分知识库健康评分**、扣分原因与可执行优化建议。

## 为什么万级文档 RAG 容易混杂

当文档规模达到万级时，不同项目、版本、业务线的 chunk 会因语义相似被一起召回，导致：

1. 正确 chunk 没召回
2. 召回了但排序靠后
3. TopK 混入无关 chunk
4. 跨 collection 污染
5. 错误版本文档污染

用户往往只能感知「回答不准」。V14.0 用可解释指标定位根因。

## Gold Test Set（核心）

自建评测集是 V14 核心。每条 case 支持（均可选，缺失则对应指标 `UNAVAILABLE`）：

| 字段 | 说明 |
|------|------|
| `query` | 必填 |
| `query_type` | 问题类型（版本限定、无答案等） |
| `collection_id` | 期望检索范围 |
| `expected_doc_ids` / `expected_chunk_ids` | 标注正确答案 |
| `negative_doc_ids` | 不应召回的文档 |
| `expected_answer_points` | 生成评测要点 |
| `metadata_filter` | 版本 / 项目过滤条件 |

导入 API：`POST /rag/evaluation/datasets/{datasetId}/cases/import`（JSON 必填，CSV 基础字段可选）。

## 公开测试集定位

T2Ranking / DuReader-Retrieval / CRUD-RAG 子集类型仅作**可选导入格式参考**，产品核心仍是自建 Gold Test Set。

## 检索策略

| 策略 | 说明 |
|------|------|
| `VECTOR_ONLY` | 纯向量检索 |
| `BM25_ONLY` | 简单 lexical 检索（非 Elasticsearch BM25） |
| `VECTOR_WITH_METADATA_FILTER` | 向量 + 应用层 metadata 过滤 |
| `HYBRID` | 向量 + lexical 候选合并 |
| `HYBRID_RRF` | RRF 融合，`score = Σ 1/(k+rank)`，默认 k=60 |
| `HYBRID_RRF_RERANK` | RRF 后接 `Reranker` 接口（默认 lexical overlap mock） |
| `HYBRID_RRF_RERANK_PARENT_CONTEXT` | 相邻 chunk 回填（无 parentChunkId 时降级并告警） |

## 检索指标

- **Recall@K**：正确 chunk 召回比例
- **HitRate@K**：TopK 是否至少命中一个正确结果
- **MRR@K**：第一个正确结果的倒数排名
- **NDCG@K**：多相关结果排序质量（二值 relevance）
- **ContextPrecision@K**：TopK 中相关 chunk 占比
- **CrossCollectionLeakRate**：指定 collection 查询时，TopK 来自错误 collection 的比例
- **WrongVersionLeakRate**：`metadata_filter.version` 指定时，TopK 来自错误版本的比例

缺失标注数据时指标标记 `available=false`，**不伪造分数**。

## 生成指标（基础版，无 LLM judge）

- **AnswerCoverage**：关键词/短语覆盖 `expected_answer_points`
- **CitationAccuracy**：引用是否来自 expected doc/chunk
- **Faithfulness**：启发式（`heuristic=true`），非严格事实一致性
- **RefusalAccuracy**：`NO_ANSWER` case 是否正确拒答
- **AnswerRelevance**：query 与 answer 关键词重合（启发式）

## RAG Quality Score

四种评分模式（权重见 `RagHealthScoringProfile`）：

- **平衡模式**（默认）
- **精准模式**：偏重 ContextPrecision、隔离指标
- **全面模式**：偏重 Recall、NDCG
- **生成可信模式**：偏重 Faithfulness、Citation、AnswerCoverage

leak rate 类指标评分时使用 `1 - rate` 转换。加权后再应用 Veto Rule 上限约束。

## Veto Rule 熔断

| 条件 | 分数上限 |
|------|----------|
| CrossCollectionLeakRate > 30% | 70 |
| WrongVersionLeakRate > 25% | 75 |
| Faithfulness < 0.5 | 65 |
| Recall@K < 0.4 | 60 |
| CitationAccuracy < 0.5 | 70 |

## 诊断与优化建议

`KnowledgeBaseDiagnosisService` 将低分指标映射到具体动作：chunk 优化、metadata filter、hybrid search、rerank、version filter、prompt 约束等。

## 优化前后对比

`POST /rag/evaluation/runs/compare` 对比两次 run 的 overallScore、各指标 delta、改善/恶化项。

## 页面

`/knowledge-health.html` — 知识库体检报告：配置评测、查看健康分、指标拆解、问题、建议、case 明细、策略对比。

## 边界（当前不做）

- OCR / 表格结构还原 / AST 解析
- LLM-as-a-judge（默认）
- 自动调参 / 自动重建知识库
- 语义近似去重 / 知识图谱

## FAQ

**Q: 为什么没有分数？**  
A: 评测 case 缺少必要标注字段，相关指标为 UNAVAILABLE，不参与严格评分。

**Q: BM25_ONLY 是真实 BM25 吗？**  
A: 否，当前为 `SimpleLexicalRetriever` 词重叠检索，测试环境使用 in-memory mock。

**Q: 会影响 V11 单次问答评分吗？**  
A: 不会。V11 `RagQualityService` 与 V14 `kbhealth` 包独立。

**Q: 如何对比 Hybrid 与 Vector Only？**  
A: 对同一评测集分别以不同 strategy 创建两次 run，再在策略对比区或 compare API 查看差异。
