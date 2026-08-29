package com.tuoman.ai_task_orchestrator.queryunderstanding;

public enum RetrievalRoutingStrategy {
    VECTOR_WITH_METADATA_FILTER,
    HYBRID_RRF,
    HYBRID_RRF_RERANK,
    HYBRID_RRF_RERANK_PARENT_CONTEXT
}
