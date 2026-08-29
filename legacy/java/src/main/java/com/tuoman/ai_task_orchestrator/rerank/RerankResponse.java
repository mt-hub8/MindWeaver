package com.tuoman.ai_task_orchestrator.rerank;

import java.util.List;

public record RerankResponse(
        List<RerankedItem> items,
        String rerankerName,
        long latencyMs
) {
}
