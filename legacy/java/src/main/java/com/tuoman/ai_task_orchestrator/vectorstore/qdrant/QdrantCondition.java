package com.tuoman.ai_task_orchestrator.vectorstore.qdrant;

public record QdrantCondition(
        String key,
        QdrantMatch match
) {
}
