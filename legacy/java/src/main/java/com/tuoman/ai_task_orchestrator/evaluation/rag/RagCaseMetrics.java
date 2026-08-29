package com.tuoman.ai_task_orchestrator.evaluation.rag;

public record RagCaseMetrics(
        boolean hit,
        double recallAtK,
        double precisionAtK,
        double rrAtK
) {
}
