# RAG 质量评分与诊断（V11.0）

## 目标

V11.0 将现有 RAG 检索评估能力产品化为「回答质量评分与诊断系统」。用户在 **知识库问答（Ask）** 页面提问后，除回答与引用外，还可获得：

- 综合评分（0–100）与等级（优秀 / 良好 / 一般 / 较差）
- 四个维度分：检索、上下文、回答、引用
- 中文诊断摘要、主要扣分原因、可执行优化建议
- 评分模式选择与可折叠的技术详情

## 为什么需要 RAG 评分

RAG 链路包含检索、上下文组装、生成与引用多个环节。仅看最终回答难以判断问题出在「没找到资料」「上下文噪声多」还是「引用不足」。质量评分帮助用户：

1. 快速判断本次回答是否可信；
2. 定位主要短板；
3. 按建议优化知识库与检索参数。

## 检索指标 vs 回答指标

| 类型 | 含义 | 典型指标 |
|------|------|----------|
| **检索指标** | 是否找到应找的内容 | Recall@K、HitRate@K、MRR、NDCG@K |
| **上下文指标** | 检索结果是否集中、少噪声 | ContextPrecision@K、Precision@K |
| **回答指标** | 回答是否回应问题、是否基于上下文 | 启发式关键词重叠、是否 no-context fallback |
| **引用指标** | 来源是否充足、可追溯 | citations 数量与字段完整性 |

离线评估（Evaluation）侧重检索指标；Ask 页面的质量评分覆盖完整问答链路。

## 综合评分如何计算

```
overallScore =
  retrievalScore × retrievalWeight
+ contextScore × contextWeight
+ answerScore × answerWeight
+ citationScore × citationWeight
```

结果四舍五入为 **0–100 整数**。

### 等级划分

| 分数 | 等级 | 中文展示 |
|------|------|----------|
| ≥ 85 | EXCELLENT | 优秀 |
| ≥ 70 | GOOD | 良好 |
| ≥ 50 | FAIR | 一般 |
| < 50 | POOR | 较差 |

## 三种评分模式

在 Ask 请求中可传 `qualityMode`（不传默认 `BALANCED`）：

### 平衡模式（BALANCED）

适合普通知识库问答。

| 维度 | 权重 |
|------|------|
| 检索 | 0.30 |
| 上下文 | 0.25 |
| 回答 | 0.25 |
| 引用 | 0.20 |

### 精准模式（PRECISE）

适合合同、技术文档、论文、规则类文档，更重视上下文精度与回答忠实度。

| 维度 | 权重 |
|------|------|
| 检索 | 0.20 |
| 上下文 | 0.30 |
| 回答 | 0.30 |
| 引用 | 0.20 |

### 全面模式（COMPREHENSIVE）

适合调研、总结、资料汇总，更重视召回覆盖面。

| 维度 | 权重 |
|------|------|
| 检索 | 0.40 |
| 上下文 | 0.20 |
| 回答 | 0.20 |
| 引用 | 0.20 |

## 各维度分含义

1. **检索质量分（retrievalScore）** — 是否找到相关内容。有标注时用 Recall@K 等；否则用 finalContextCount、top 分数、命中数量等启发式。
2. **上下文质量分（contextScore）** — 上下文是否集中相关。有标注时用 ContextPrecision@K；否则看分数分布、片段数量、query 与 snippet 重合度。
3. **回答质量分（answerScore）** — **启发式估算**，非 LLM judge：检查是否空回答、是否 no-context 话术、关键词相关性等。
4. **引用质量分（citationScore）** — citations 数量、documentId/chunkId/snippet 完整性、与回答是否匹配。

## 诊断规则

| 代码 | 条件 | 说明 |
|------|------|------|
| NO_CONTEXT | finalContextCount = 0 | 未检索到可用上下文 |
| LOW_RETRIEVAL_RECALL | retrievalScore < 60 | 检索召回不足 |
| LOW_CONTEXT_PRECISION | contextScore < 60 且有条目 | 可能混入噪声 |
| WEAK_CITATION | citationScore < 60 | 引用不足或不完整 |
| ANSWER_NOT_GROUNDED_RISK | 有确定回答但无引用/无上下文 | 可能缺少知识库依据 |
| EMBEDDING_MODEL_SWITCH_RISK | 索引维度与当前 embedding 不一致 | 需重新索引 |
| SLOW_RESPONSE | latencyMs ≥ 15s / 30s | 耗时较长 |

## 常见低分原因

- 知识库无相关文档或未选正确分组
- topK 过小或过大
- 文档未索引或 embedding 模型变更后未重新索引
- 检索命中但片段与问题关键词重合低
- 无上下文时仍生成看似确定的回答

## 如何根据建议优化

1. **补文档** — 上传与问题主题更接近的资料；
2. **调范围** — 选择正确 Collection 或改用「全部文档」；
3. **调 topK** — 召回不足可提高，噪声多可降低；
4. **换模式** — 调研类用全面模式，规则类用精准模式；
5. **重新索引** — 更换 embedding 模型后务必重新索引；
6. **性能** — 降低 topK、缩小范围或使用更快模型。

## 为什么默认不使用真实 LLM judge

- 避免额外外部 API 依赖与成本；
- 保证默认 `mvn test` 不调用真实 LLM；
- 评分逻辑可解释、可复现；
- 回答质量当前为**启发式估算**，不声称严格事实一致性评估。

## 当前局限

- 无标准答案时，检索/上下文分亦为启发式；
- 不做自动调参、自动重新索引；
- 不支持用户自定义权重持久化（V11.0 仅请求级 `qualityMode`）；
- 不做在线 A/B 与人工标注平台。

## API 示例

```http
POST /rag/answers
Content-Type: application/json

{
  "query": "这个系统的 Java 和 Python 分别负责什么？",
  "topK": 5,
  "collectionId": 1,
  "qualityMode": "BALANCED"
}
```

响应含 `qualityScore` 对象，包括 `overallScore`、`diagnosis`、`weights`、`metricDetails` 等。

## 后续可扩展方向

- 结合离线评估数据集自动填充 Recall@K 等标注指标；
- 可选 rerank 后重算上下文分；
- 用户级权重配置与历史趋势；
- V12.0 垃圾箱与存储管理联动清理低质量索引。
