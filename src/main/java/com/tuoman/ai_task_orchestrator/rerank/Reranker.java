package com.tuoman.ai_task_orchestrator.rerank;

/**
 * RAG reranker 抽象。
 *
 * rerank 的输入必须是已经通过 retrieval filter 的候选集合，输出只改变候选顺序和截断数量。
 * 它不负责重新检索，也不能扩大 collection、version、status 或生命周期范围。
 */
public interface Reranker {

    RerankResponse rerank(RerankRequest request);

    String name();
}
