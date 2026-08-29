package com.tuoman.ai_task_orchestrator.vectorstore.qdrant;

import java.util.List;

public record QdrantUpsertPointsRequest(
        List<QdrantPoint> points
) {
}
