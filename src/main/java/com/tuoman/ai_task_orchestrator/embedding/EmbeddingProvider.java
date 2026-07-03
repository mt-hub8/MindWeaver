package com.tuoman.ai_task_orchestrator.embedding;

import java.util.List;

public interface EmbeddingProvider {

    EmbeddingResponse embed(EmbeddingRequest request);

    default List<EmbeddingResponse> embedBatch(List<EmbeddingRequest> requests) {
        if (requests == null || requests.isEmpty()) {
            return List.of();
        }
        return requests.stream()
                .map(this::embed)
                .toList();
    }

    String provider();

    default String runtimeProvider() {
        return provider();
    }

    String model();

    int dimension();
}
