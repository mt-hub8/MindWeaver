package com.tuoman.ai_task_orchestrator.evaluation.rag;

public record RagRetrievalComparisonCaseResult(
        String caseId,
        String query,
        int finalTopK,
        int candidateTopK,
        RagRetrievalCaseResult baseline,
        RagRetrievalCaseResult rerank,
        String outcome
) {
}
