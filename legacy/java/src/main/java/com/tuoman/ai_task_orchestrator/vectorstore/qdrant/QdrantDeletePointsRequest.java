package com.tuoman.ai_task_orchestrator.vectorstore.qdrant;

public record QdrantDeletePointsRequest(
        QdrantFilter filter
) {
}
