package com.tuoman.ai_task_orchestrator.evaluation;

import java.util.List;

public record RetrievalBenchmarkCase(
        String caseId,
        String query,
        List<String> expectedEvidenceIds
) {
}
