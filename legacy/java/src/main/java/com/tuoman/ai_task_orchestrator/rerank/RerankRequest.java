package com.tuoman.ai_task_orchestrator.rerank;

import java.util.List;

public record RerankRequest(
        String query,
        List<RerankCandidate> candidates,
        int finalTopK
) {
}
