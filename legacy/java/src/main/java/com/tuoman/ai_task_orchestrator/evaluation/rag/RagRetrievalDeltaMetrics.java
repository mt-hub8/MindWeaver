package com.tuoman.ai_task_orchestrator.evaluation.rag;

public record RagRetrievalDeltaMetrics(
        double hitRateDelta,
        double recallDelta,
        double precisionDelta,
        double mrrDelta,
        int improvedCount,
        int regressedCount,
        int unchangedCount
) {
}
