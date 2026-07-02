package com.tuoman.ai_task_orchestrator.hybrid;

import java.util.List;

public record FusionResponse(
        List<FusedCandidate> candidates,
        String fusionStrategy
) {
}
