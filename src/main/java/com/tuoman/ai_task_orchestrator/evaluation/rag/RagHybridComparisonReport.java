package com.tuoman.ai_task_orchestrator.evaluation.rag;

import java.time.Instant;
import java.util.List;

public record RagHybridComparisonReport(
        String datasetName,
        String datasetPath,
        Instant runAt,
        Integer defaultTopK,
        Integer denseTopK,
        Integer lexicalTopK,
        Integer rrfK,
        String fusionStrategy,
        boolean rerankEnabled,
        String rerankerName,
        String embeddingProvider,
        String embeddingModel,
        Integer embeddingDimension,
        String vectorStore,
        RagRetrievalSummaryMetrics baselineSummary,
        RagRetrievalSummaryMetrics hybridSummary,
        RagRetrievalDeltaMetrics delta,
        List<RagHybridComparisonCaseResult> cases
) {
}
