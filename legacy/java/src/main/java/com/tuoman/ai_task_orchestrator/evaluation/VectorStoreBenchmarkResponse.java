package com.tuoman.ai_task_orchestrator.evaluation;

import java.util.List;

public record VectorStoreBenchmarkResponse(
        String datasetId,
        VectorStoreBenchmarkSideResult baseline,
        VectorStoreBenchmarkSideResult candidate,
        List<VectorStoreMetricDelta> metricDeltas,
        long searchLatencyDeltaNanos
) {
}
