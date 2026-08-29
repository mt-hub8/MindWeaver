package com.tuoman.ai_task_orchestrator.evaluation;

public record LatencyStats(
        int searchCount,
        long totalNanos,
        double averageMillis,
        double minMillis,
        double maxMillis,
        double p50Millis,
        double p95Millis
) {

    public static LatencyStats empty() {
        return new LatencyStats(0, 0L, 0.0, 0.0, 0.0, 0.0, 0.0);
    }
}
