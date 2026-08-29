package com.tuoman.ai_task_orchestrator.vectorstore;

import java.util.List;

public record VectorSearchRequest(
        List<Double> queryEmbedding,
        Integer topK,
        String provider,
        String model,
        Integer dimension,
        VectorSearchFilter filter
) {
}
