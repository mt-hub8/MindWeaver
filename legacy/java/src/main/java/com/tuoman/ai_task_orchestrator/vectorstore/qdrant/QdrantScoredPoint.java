package com.tuoman.ai_task_orchestrator.vectorstore.qdrant;

import java.util.Map;

public record QdrantScoredPoint(
        Object id,
        Double score,
        Map<String, Object> payload
) {
}
