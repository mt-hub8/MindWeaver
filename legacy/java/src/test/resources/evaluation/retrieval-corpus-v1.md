# Transactional Outbox

[EVIDENCE:outbox-reason]
createTask 不直接发送 RabbitMQ，而是在同一个数据库事务中写入 task、task_event 和 task_outbox。这样可以避免数据库提交成功但消息发送失败，或者消息发送成功但数据库事务失败的 dual write problem。Outbox Dispatcher 后续扫描 task_outbox 并负责可靠投递消息。

# Atomic Task Claim

[EVIDENCE:atomic-claim-purpose]
Consumer 收到 RabbitMQ 消息后，不直接执行任务，而是通过数据库原子 update 将 PENDING / RETRY_PENDING claim 为 RUNNING。只有 claim 成功的 worker 才能执行任务，从而避免重复消费或多实例并发导致同一个任务被重复执行。

# Task Attempt

[EVIDENCE:task-attempt-metadata]
task_attempt 记录每次执行尝试，包括 attemptNo、status、workerId、llmProvider、llmModel、promptTokenCount、completionTokenCount、totalTokenCount、llmLatencyMs、errorMessage、startedAt 和 finishedAt。它用于执行审计、排障和后续指标分析。

# RAG Retrieval Evaluation

[EVIDENCE:retrieval-evaluation-purpose]
Retrieval Evaluation 用 query 和 expected evidence 对比 TopK 检索结果，计算 Recall@K、Precision@K、NDCG@K、MRR、HitRate@K 和 Context Precision。它用于量化检索质量，避免只能凭感觉优化 RAG。

# RAG Answer Citation

[EVIDENCE:rag-citation-purpose]
RAG Answer 将检索到的 TopK chunks 构造成 context prompt，调用 LlmClient / MockLlmClient，并返回 answer 和 chunk-level citations。citation 让答案能够追溯到来源 chunk，但当前还不是 sentence-level citation。
