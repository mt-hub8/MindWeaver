package com.tuoman.ai_task_orchestrator.evaluation;

import java.util.List;

public record RetrievalBenchmarkDataset(
        String datasetId,
        String description,
        String corpusFile,
        List<Integer> topKValues,
        List<RetrievalBenchmarkCase> cases
) {
}
