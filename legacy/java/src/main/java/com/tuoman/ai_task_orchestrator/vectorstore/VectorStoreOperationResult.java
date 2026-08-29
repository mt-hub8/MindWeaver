package com.tuoman.ai_task_orchestrator.vectorstore;

import java.util.List;

public record VectorStoreOperationResult(
        int affectedCount,
        List<String> warnings
) {
    public static VectorStoreOperationResult success(int affectedCount) {
        return new VectorStoreOperationResult(affectedCount, List.of());
    }

    public static VectorStoreOperationResult unsupported(String message) {
        return new VectorStoreOperationResult(0, List.of(message));
    }
}
