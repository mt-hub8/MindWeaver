package com.tuoman.ai_task_orchestrator.embedding;

import java.util.List;

public record CachedEmbeddingResult(
        List<Double> embedding,
        String provider,
        String model,
        int dimension,
        String distanceMetric,
        String chunkHash,
        boolean cacheHit
) {
}
