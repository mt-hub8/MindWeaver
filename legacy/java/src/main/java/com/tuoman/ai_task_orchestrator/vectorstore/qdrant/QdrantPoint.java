package com.tuoman.ai_task_orchestrator.vectorstore.qdrant;

import java.util.List;
import java.util.Map;

public record QdrantPoint(
        Object id,
        List<Double> vector,
        Map<String, Object> payload
) {
}
