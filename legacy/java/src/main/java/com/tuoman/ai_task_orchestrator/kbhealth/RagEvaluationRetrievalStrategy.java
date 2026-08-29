package com.tuoman.ai_task_orchestrator.kbhealth;

public enum RagEvaluationRetrievalStrategy {
    VECTOR_ONLY,
    BM25_ONLY,
    VECTOR_WITH_METADATA_FILTER,
    HYBRID,
    HYBRID_RRF,
    HYBRID_RRF_RERANK,
    HYBRID_RRF_RERANK_PARENT_CONTEXT
}
