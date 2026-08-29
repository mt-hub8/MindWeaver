# Qdrant Benchmark 结果采集指南（V2.6.7）

本文说明如何运行 **ExactCosineVectorStore vs QdrantVectorStore** 的手工 benchmark，并将真实运行结果保存为 JSON 与 Markdown 摘要。

> **重要**：本 benchmark 是 **manual benchmark**，不是默认 CI test。`.\mvnw.cmd test` 默认**不会**连接 Qdrant、不会启动 Python worker、不会下载模型。

---

## 1. 本版本目标

在已有 VectorStore Benchmark Harness 之上，建立一套可复现的手工流程：

- 对比 baseline（`ExactCosineVectorStore`）与 candidate（`QdrantVectorStore`）
- 复用已有 retrieval benchmark dataset 与 evaluation metrics
- 输出 `build/reports/retrieval/qdrant-benchmark-result.json`
- 输出 `build/reports/retrieval/qdrant-benchmark-summary.md`
- 记录 dataset、topK、provider、model、dimension、latency、generatedAt 等元数据

核心目标：让 exact vs qdrant benchmark 从「可以跑」变成「可以采集、保存、解释、复现」。

---

## 2. 本 benchmark 能说明什么

| 能说明 | 说明 |
| --- | --- |
| 检索质量对比 | 在同一 corpus、同一 query cases、同一 topK、同一 embedding provider/model/dimension 下，两种 VectorStore 的 Recall@K、Precision@K、MRR、NDCG@K、ContextPrecision@K 等指标差异 |
| 搜索延迟观测 | 在当前机器、当前 Qdrant Docker、当前 HTTP 客户端配置下，每次 query search 的 avg / min / max / p50 / p95 延迟 |
| 可复现性 | 固定 dataset 文件 + 显式 Maven 参数 + 独立 Qdrant collection，便于重复运行并对比产物 |

---

## 3. 本 benchmark 不能说明什么

| 不能说明 | 原因 |
| --- | --- |
| Qdrant 一定更快 | 小数据集下 exact 全量余弦可能更快；Qdrant 还有 HTTP、序列化、Docker 开销 |
| 生产环境性能结论 | 未模拟真实流量、副本、网络拓扑、索引调优 |
| 真实 embedding 质量 | 默认使用 `MockEmbeddingClient`，不下载真实模型 |
| 混合检索 / Rerank / Hybrid | 本版本仅做向量检索对比 |
| CI 回归门禁 | 默认 `mvn test` 跳过本测试 |

**请勿编造「Qdrant 一定更快」的结论。** 指标必须来自真实运行产物，不能手写数值。

---

## 4. 前置条件

- JDK 21+
- Docker Desktop（用于 Qdrant）
- 项目可正常执行 `.\mvnw.cmd test`（默认测试通过，且不依赖外部服务）
- 可选：若改用 `local-worker` 真实 embedding，需按 `workers/embedding-worker/README.md` 启动 worker（本 benchmark 默认 **不需要**）

进入项目目录：

```powershell
cd E:\code\ai-task-orchestrator
```

---

## 5. 启动 Qdrant

使用独立 compose 文件（不修改主 `docker-compose.yml`）：

```powershell
cd E:\code\ai-task-orchestrator
docker compose -f docker-compose.qdrant.yml up -d
docker compose -f docker-compose.qdrant.yml ps
```

| 项 | 值 |
| --- | --- |
| HTTP API | `http://127.0.0.1:6333` |
| 默认 image | `qdrant/qdrant:v1.12.5` |

健康检查：

```powershell
Invoke-WebRequest -Uri http://127.0.0.1:6333/healthz -UseBasicParsing
```

---

## 6. 可选：启动 Local Embedding Worker

本 benchmark **默认使用 `MockEmbeddingClient`**（provider=`mock`，dimension=128），**无需**启动 Python worker，也**不会**下载模型。

仅当你自行修改测试以使用 `local-worker` 真实 embedding 时，才需要启动 worker。参见 `workers/embedding-worker/README.md`。

---

## 7. 运行 manual benchmark

### 7.1 启用方式

测试类：`QdrantVectorStoreManualBenchmarkTest`

必须通过 JVM system property 显式启用：

```powershell
cd E:\code\ai-task-orchestrator
.\mvnw.cmd -Dqdrant.manual.benchmark=true -Dtest=QdrantVectorStoreManualBenchmarkTest test
```

### 7.2 可选参数

| System property | 默认值 | 说明 |
| --- | --- | --- |
| `qdrant.manual.benchmark` | （未设置） | 必须为 `true` 才运行 |
| `qdrant.manual.benchmark.base-url` | `http://127.0.0.1:6333` | Qdrant HTTP 地址 |
| `qdrant.manual.benchmark.output-dir` | `build/reports/retrieval` | 结果输出目录 |

示例（自定义输出目录）：

```powershell
.\mvnw.cmd -Dqdrant.manual.benchmark=true -Dqdrant.manual.benchmark.output-dir=build/reports/retrieval -Dtest=QdrantVectorStoreManualBenchmarkTest test
```

### 7.3 对比设置

| 角色 | 实现 |
| --- | --- |
| Baseline | `ExactCosineVectorStore` |
| Candidate | `QdrantVectorStore` |
| Dataset corpus | `src/test/resources/evaluation/retrieval-corpus-v1.md` |
| Dataset cases | `src/test/resources/evaluation/retrieval-benchmark-v1.json` |
| Evidence 映射 | `BenchmarkEvidenceMapper` |
| Metrics | `RetrievalMetricsCalculator`（经 `RetrievalEvaluationService`） |
| Embedding（默认） | `MockEmbeddingClient` |

每次运行会为 Qdrant 创建**唯一 collection 名称**（`ai_task_orchestrator_benchmark_<uuid>`），避免混入其他 provider/model/dimension 的向量。

---

## 8. 输出文件位置

运行成功后，产物位于：

| 文件 | 路径 |
| --- | --- |
| JSON 结果 | `build/reports/retrieval/qdrant-benchmark-result.json` |
| Markdown 摘要 | `build/reports/retrieval/qdrant-benchmark-summary.md` |

> **请勿将 `build/reports/` 下的运行产物提交到 Git。** 它们是本地观测记录，不是源码。

---

## 9. JSON result 字段说明

```json
{
  "benchmarkName": "exact-vs-qdrant",
  "generatedAt": "ISO-8601 时间戳",
  "dataset": {
    "corpus": "retrieval-corpus-v1.md",
    "cases": "retrieval-benchmark-v1.json",
    "caseCount": 5,
    "topK": 5,
    "topKValues": [1, 3, 5]
  },
  "embedding": {
    "provider": "mock",
    "model": "mock-embedding-v1",
    "dimension": 128
  },
  "vectorStores": {
    "baseline": "ExactCosineVectorStore",
    "candidate": "QdrantVectorStore"
  },
  "qdrant": {
    "baseUrl": "http://127.0.0.1:6333",
    "collectionName": "ai_task_orchestrator_benchmark_...",
    "distance": "Cosine"
  },
  "baseline": {
    "summaryMetrics": [ { "k": 1, "recallAtK": "...", "...": "..." } ],
    "latency": {
      "searchCount": 5,
      "totalNanos": "...",
      "averageMillis": "...",
      "minMillis": "...",
      "maxMillis": "...",
      "p50Millis": "...",
      "p95Millis": "..."
    }
  },
  "candidate": { "... 同上结构 ..." },
  "delta": {
    "searchLatencyDeltaNanos": "...",
    "searchLatencyDeltaMillis": "...",
    "metricsByK": [ { "k": 1, "recallAtK": "...", "...": "..." } ]
  },
  "notes": [
    "This result is from a local manual benchmark run and must not be interpreted as a production performance claim."
  ]
}
```

所有指标数值均来自 `VectorStoreBenchmarkRunner` 的真实输出，不是硬编码。

---

## 10. Markdown summary 字段说明

Markdown 文件包含：

- Benchmark 名称与 `generatedAt`
- Dataset、case count、topK values
- Embedding provider / model / dimension
- Baseline 与 candidate 名称
- **Summary Metrics 表格**（baseline 与 candidate 各一张）
- **Search Latency 表格**（avg / min / max / p50 / p95）
- **Metric Delta 表格**（candidate − baseline）
- 运行环境说明
- **限制说明**（本地手工运行、不代表生产性能）

---

## 11. 如何解读检索指标

以下指标由 `RetrievalMetricsCalculator` 计算，在 benchmark 中对每个 topK 汇总：

| 指标 | 中文 | 含义（简要） |
| --- | --- | --- |
| Recall@K | 召回率@K | 期望证据 chunk 中，有多少比例出现在前 K 个检索结果里 |
| Precision@K | 精确率@K | 前 K 个结果里，有多少比例是期望证据 chunk |
| HitRate@K | 命中率@K | 是否至少命中一个期望 chunk（0 或 1，汇总为平均） |
| MRR | 平均倒数排名 | 第一个正确结果排名的倒数的平均值 |
| NDCG@K | 归一化折损累计增益@K | 考虑排名位置的排序质量 |
| ContextPrecision@K | 上下文精确率@K | 前 K 结果中相关 chunk 的精确率（本项目 evaluation 定义） |

**Delta 列** = candidate 指标 − baseline 指标。正值表示 candidate 更高（需结合具体指标语义判断好坏）。

---

## 12. 如何理解 latency

Latency 由 `LatencyMeasuringVectorStore` 包装 delegate VectorStore，在每次 `search()` 调用前后计时（纳秒），并汇总为：

| 字段 | 说明 |
| --- | --- |
| `searchCount` | 执行的 search 次数（等于 benchmark case 数） |
| `averageMillis` | 平均单次 search 耗时（毫秒） |
| `minMillis` / `maxMillis` | 最小 / 最大单次耗时 |
| `p50Millis` / `p95Millis` | 50 / 95 分位耗时 |

注意事项：

- 这是**测试 harness 内**的观测值，包含 JVM、HTTP 客户端、Qdrant Docker 等开销
- 小数据集下 baseline exact search 可能比 Qdrant 更快，**属于正常现象**
- 不要用单次手工运行的 latency 推导生产 SLA

---

## 13. 为什么默认 test 不运行 Qdrant benchmark

| 原因 | 说明 |
| --- | --- |
| CI / 本地开发隔离 | 默认 `mvn test` 不应依赖 Docker、外部网络、Qdrant |
| 可重复性 | 无 Qdrant 时仍可验证 harness、report writer、metrics 逻辑 |
| 显式 opt-in | `@EnabledIfSystemProperty(named = "qdrant.manual.benchmark", matches = "true")` |

默认仍会运行 `VectorStoreBenchmarkComparisonTest`（exact vs 内存 fake candidate）和 `VectorStoreBenchmarkReportWriterTest`（无 Qdrant）。

---

## 14. 常见问题

### Q: `mvn test` 报 Qdrant 连接失败？

A: 不应发生。确认未设置 `-Dqdrant.manual.benchmark=true`。仅手工命令需要 Qdrant。

### Q: manual benchmark 失败 `Connection refused`？

A: 先执行 `docker compose -f docker-compose.qdrant.yml up -d`，确认 `6333` 可访问。

### Q: baseline 与 candidate 指标完全一致？

A: 在相同 embedding、相同 ranking 逻辑下，检索质量应接近；若 Qdrant 过滤或 payload 映射有误，可能出现差异，需结合 case 明细排查。

### Q: Qdrant 比 exact 慢很多？

A: 小数据 + HTTP + Docker 开销常见。这不等于 Qdrant 在生产大规模数据上更慢。

### Q: 需要 OpenAI API key 吗？

A: 默认不需要。使用 `MockEmbeddingClient`。

### Q: 输出目录没有文件？

A: 确认测试通过且使用了正确的 `-Dtest=QdrantVectorStoreManualBenchmarkTest`。检查 `build/reports/retrieval/` 或自定义 `output-dir`。

---

## 15. 清理 Qdrant

停止容器：

```powershell
docker compose -f docker-compose.qdrant.yml down
```

删除数据卷（清空所有 collection 数据）：

```powershell
docker compose -f docker-compose.qdrant.yml down -v
```

手工 benchmark 每次使用独立 collection 名称；长期积累可定期 `down -v` 或调用 Qdrant API 删除指定 collection：

```powershell
Invoke-WebRequest -Method DELETE -Uri "http://127.0.0.1:6333/collections/<collectionName>" -UseBasicParsing
```

---

## 16. 手工验收流程（Checklist）

1. 启动 Qdrant：`docker compose -f docker-compose.qdrant.yml up -d`
2. （可选）若使用真实 embedding，启动 local worker
3. 运行 benchmark：
   ```powershell
   .\mvnw.cmd -Dqdrant.manual.benchmark=true -Dtest=QdrantVectorStoreManualBenchmarkTest test
   ```
4. 查看输出：
   - `build/reports/retrieval/qdrant-benchmark-result.json`
   - `build/reports/retrieval/qdrant-benchmark-summary.md`
5. 确认默认测试仍隔离：
   ```powershell
   .\mvnw.cmd test
   ```

---

## 17. 相关代码

| 组件 | 路径 |
| --- | --- |
| Manual benchmark test | `src/test/java/.../evaluation/QdrantVectorStoreManualBenchmarkTest.java` |
| Benchmark runner | `src/main/java/.../evaluation/VectorStoreBenchmarkRunner.java` |
| Report writer | `src/main/java/.../evaluation/VectorStoreBenchmarkReportWriter.java` |
| Latency 包装 | `src/main/java/.../evaluation/LatencyMeasuringVectorStore.java` |
| Dataset | `src/test/resources/evaluation/retrieval-corpus-v1.md`、`retrieval-benchmark-v1.json` |
