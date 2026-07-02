package com.tuoman.ai_task_orchestrator.hybrid;

public record DenseCandidate(
        int rank,
        Long documentId,
        String documentTitle,
        Long chunkId,
        String content,
        Double denseScore
) {
}
