package com.tuoman.ai_task_orchestrator.hybrid;

import java.util.List;

public record LexicalRetrievalResponse(
        List<LexicalCandidate> candidates,
        long latencyMs
) {
}
