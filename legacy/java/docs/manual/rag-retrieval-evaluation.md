# RAG Retrieval Evaluation Manual（V2.8）

本文说明如何使用 V2.8 的离线评估能力，批量评估 RAG 检索质量，并输出 JSON/Markdown 报告。

## 1. V2.8 目标

- 从 evaluation dataset 加载多条检索 case
- 对每个 case 执行 query embedding + VectorStore TopK 检索
- 按 expected items 规则匹配 relevant 结果
- 计算 HitRate@K、Recall@K、Precision@K、RR@K，并聚合 MRR
- 输出 per-case result + summary metrics

## 2. Dataset 结构

默认数据集路径：`docs/evaluation/rag-retrieval-eval-cases.json`

`expectedItems` 支持稳定匹配字段（可组合）：

- `expectedChunkId`（可选）
- `documentId`（可选）
- `documentTitle`（可选）
- `chunkContains`（可选）

至少要提供一个匹配字段。

## 3. 指标定义

- `HitRate@K`：TopK 至少命中 1 个 expected item，则该 case 命中。
- `Recall@K`：`matched expected count / total expected count`。
- `Precision@K`：`matched expected count / retrieved count`。
- `RR@K`：第一个 relevant 结果排名为 r，则 `RR = 1/r`，未命中为 0。
- `MRR`：所有 case 的 `RR@K` 平均值。

## 4. 运行方式（真实评估，显式 opt-in）

### 4.1 前置依赖

- Qdrant（手工启动）
- Local Embedding Worker（手工启动）
- Spring Boot 建议使用 `docker,qdrant` profile
- demo 文档已上传并完成 embeddings

### 4.2 启动命令

```powershell
cd E:\code\ai-task-orchestrator
.\mvnw.cmd spring-boot:run "-Dspring-boot.run.profiles=docker,qdrant" "-Dspring-boot.run.arguments=--app.embedding.provider=local-worker --app.evaluation.retrieval.enabled=true --app.evaluation.retrieval.document-id=1"
```

可选参数：

- `app.evaluation.retrieval.dataset-path`
- `app.evaluation.retrieval.report-output-dir`
- `app.evaluation.retrieval.default-top-k`
- `app.evaluation.retrieval.document-id`

Runner 只会在 `app.evaluation.retrieval.enabled=true` 时执行。

## 5. 报告路径

- JSON：`docs/evaluation/reports/rag-retrieval-evaluation-report.json`
- Markdown：`docs/evaluation/reports/rag-retrieval-evaluation-report.md`

## 6. 为什么默认 `.\mvnw.cmd test` 不跑真实 evaluation

默认测试必须保持低依赖边界，不依赖 Qdrant/Docker/worker/外部网络/模型下载。  
真实评估属于手工验证场景，必须显式 opt-in。

## 7. 常见失败排查

- Qdrant 未启动：检查 `6333` 与 `qdrant` profile。
- Local worker 未启动：检查 `8001/health`。
- Spring profile 错误：确认 `docker,qdrant` 已生效。
- 维度不匹配：确保 embedding 与 collection 在同一向量空间（例如 local-worker 384）。
- 数据未摄入：确认先执行上传与 embeddings，再跑评估。
- 报告为空或全 0：确认 `document-id` 正确，query 与语料相关。
