package com.tuoman.ai_task_orchestrator.retrieval;

public enum CollectionAskEmptyReason {
    NONE,
    COLLECTION_NOT_FOUND,
    NO_DOCUMENTS,
    ALL_DELETED,
    NO_RETRIEVABLE_CHUNKS
}
