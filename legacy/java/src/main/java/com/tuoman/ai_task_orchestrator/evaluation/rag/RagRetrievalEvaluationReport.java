package com.tuoman.ai_task_orchestrator.evaluation.rag;

import java.time.Instant;
import java.util.List;

public record RagRetrievalEvaluationReport(
        String datasetName,
        String datasetPath,
        Instant runAt,
        Integer defaultTopK,
        String embeddingProvider,
        String embeddingModel,
        Integer embeddingDimension,
        String vectorStore,
        RagRetrievalSummaryMetrics summary,
        List<RagRetrievalCaseResult> cases
) {
}
