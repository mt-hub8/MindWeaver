package com.tuoman.ai_task_orchestrator.evaluation.rag;

import java.util.List;

public record RagRetrievalEvaluationCase(
        String caseId,
        String query,
        Integer topK,
        List<RagRetrievalExpectedItem> expectedItems
) {
}
