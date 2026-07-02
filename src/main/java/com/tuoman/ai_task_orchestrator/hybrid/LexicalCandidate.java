package com.tuoman.ai_task_orchestrator.hybrid;

public record LexicalCandidate(
        int rank,
        Long documentId,
        String documentTitle,
        Long chunkId,
        String content,
        double lexicalScore
) {
}
