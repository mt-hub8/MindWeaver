package com.tuoman.ai_task_orchestrator.evaluation.rag;

public record RagRetrievalSummaryMetrics(
        int totalCases,
        int hitCount,
        double hitRateAtK,
        double averageRecallAtK,
        double averagePrecisionAtK,
        double mrr,
        double averageLatencyMs
) {
}
