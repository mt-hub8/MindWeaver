package com.tuoman.ai_task_orchestrator.evaluation.rag;

public record RagHybridComparisonCaseResult(
        String caseId,
        String query,
        int topK,
        int denseTopK,
        int lexicalTopK,
        RagRetrievalCaseResult baseline,
        RagRetrievalCaseResult hybrid,
        String outcome
) {
}
