package com.tuoman.ai_task_orchestrator.evaluation.rag;

public record RagRetrievalExpectedItem(
        String expectedId,
        Long documentId,
        String documentTitle,
        Long expectedChunkId,
        String chunkContains
) {
}
