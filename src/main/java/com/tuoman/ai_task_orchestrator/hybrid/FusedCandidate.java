package com.tuoman.ai_task_orchestrator.hybrid;

public record FusedCandidate(
        int fusionRank,
        Long documentId,
        String documentTitle,
        Long chunkId,
        String content,
        Integer denseRank,
        Integer lexicalRank,
        Double denseScore,
        Double lexicalScore,
        double fusionScore,
        boolean denseHit,
        boolean lexicalHit
) {
}
