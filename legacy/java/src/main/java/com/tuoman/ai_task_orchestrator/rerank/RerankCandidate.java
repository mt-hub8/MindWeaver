package com.tuoman.ai_task_orchestrator.rerank;

public record RerankCandidate(
        int originalRank,
        Long documentId,
        String documentTitle,
        Long chunkId,
        String content,
        Double originalScore
) {
}
