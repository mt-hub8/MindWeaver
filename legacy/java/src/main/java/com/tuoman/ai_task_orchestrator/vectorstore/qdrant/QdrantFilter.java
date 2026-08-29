package com.tuoman.ai_task_orchestrator.vectorstore.qdrant;

import java.util.List;

public record QdrantFilter(
        List<QdrantCondition> must
) {
}
