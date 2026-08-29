package com.tuoman.ai_task_orchestrator.evaluation.rag;

import java.util.List;

public record RagRetrievalCaseResult(
        String caseId,
        String query,
        int topK,
        List<RagRetrievalExpectedItem> expectedItems,
        List<RagRetrievedItem> retrievedItems,
        List<RagRetrievalExpectedItem> matchedExpectedItems,
        boolean hit,
        double recallAtK,
        double precisionAtK,
        double rrAtK,
        long latencyMs
) {
}
