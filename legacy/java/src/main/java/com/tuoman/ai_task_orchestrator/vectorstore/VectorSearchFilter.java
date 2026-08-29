package com.tuoman.ai_task_orchestrator.vectorstore;

import java.util.List;
import java.util.Map;

public record VectorSearchFilter(
        List<Long> documentIds,
        Map<String, String> metadataEquals
) {

    public static VectorSearchFilter empty() {
        return new VectorSearchFilter(List.of(), Map.of());
    }
}
