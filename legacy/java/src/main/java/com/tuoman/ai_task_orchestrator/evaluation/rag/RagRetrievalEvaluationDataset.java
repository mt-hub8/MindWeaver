package com.tuoman.ai_task_orchestrator.evaluation.rag;

import java.util.List;

public record RagRetrievalEvaluationDataset(
        String datasetName,
        String description,
        Integer defaultTopK,
        List<RagRetrievalEvaluationCase> cases
) {
}
