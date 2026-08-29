package com.tuoman.ai_task_orchestrator.vectorstore;

import java.util.Map;

public record VectorSearchResult(
        Long chunkId,
        Long documentId,
        Integer chunkIndex,
        String content,
        Integer contentLength,
        String headingPath,
        Integer startOffset,
        Integer endOffset,
        String chunkStrategy,
        Double score,
        Integer rank,
        String provider,
        String model,
        Integer dimension,
        String distanceMetric,
        Map<String, String> metadata
) {
}
