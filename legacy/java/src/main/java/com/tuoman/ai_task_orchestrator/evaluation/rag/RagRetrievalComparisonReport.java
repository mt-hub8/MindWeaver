package com.tuoman.ai_task_orchestrator.evaluation.rag;

import java.time.Instant;
import java.util.List;

public record RagRetrievalComparisonReport(
        String datasetName,
        String datasetPath,
        Instant runAt,
        Integer defaultTopK,
        Integer candidateTopK,
        String embeddingProvider,
        String embeddingModel,
        Integer embeddingDimension,
        String vectorStore,
        String rerankerName,
        RagRetrievalSummaryMetrics baselineSummary,
        RagRetrievalSummaryMetrics rerankSummary,
        RagRetrievalDeltaMetrics delta,
        List<RagRetrievalComparisonCaseResult> cases
) {
}
