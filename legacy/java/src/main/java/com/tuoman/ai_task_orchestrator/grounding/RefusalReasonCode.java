package com.tuoman.ai_task_orchestrator.grounding;

public enum RefusalReasonCode {
    NO_CONTEXT,
    LOW_RETRIEVAL_CONFIDENCE,
    NO_VALID_CITATION,
    QUERY_AMBIGUOUS,
    VERSION_NOT_FOUND,
    COLLECTION_NOT_SELECTED,
    STRICT_MODE_NO_EVIDENCE,
    NONE
}
