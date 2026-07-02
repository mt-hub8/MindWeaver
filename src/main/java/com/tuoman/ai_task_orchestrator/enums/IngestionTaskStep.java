package com.tuoman.ai_task_orchestrator.enums;

public enum IngestionTaskStep {
    UPLOADED,
    TEXT_EXTRACTED,
    CHUNKING,
    EMBEDDING,
    VECTOR_WRITING,
    COMPLETED,
    FAILED
}
