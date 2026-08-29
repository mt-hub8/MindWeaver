package com.tuoman.ai_task_orchestrator.rerank;

public record RerankedItem(
        int rerankedRank,
        int originalRank,
        Long documentId,
        String documentTitle,
        Long chunkId,
        String content,
        Double originalScore,
        double rerankScore
) {
}
