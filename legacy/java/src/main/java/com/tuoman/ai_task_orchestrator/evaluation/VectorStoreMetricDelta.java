package com.tuoman.ai_task_orchestrator.evaluation;

public record VectorStoreMetricDelta(
        Integer k,
        Double recallAtKDelta,
        Double precisionAtKDelta,
        Double hitRateAtKDelta,
        Double mrrDelta,
        Double ndcgAtKDelta,
        Double contextPrecisionAtKDelta
) {
}
