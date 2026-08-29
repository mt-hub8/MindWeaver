package com.tuoman.ai_task_orchestrator.hybrid;

import java.util.List;

public record FusionRequest(
        List<DenseCandidate> denseCandidates,
        List<LexicalCandidate> lexicalCandidates
) {
}
