package com.tuoman.ai_task_orchestrator.evaluation;

import com.tuoman.ai_task_orchestrator.dto.RetrievalEvaluationCaseResultResponse;
import com.tuoman.ai_task_orchestrator.dto.RetrievalEvaluationSummaryResponse;

import java.util.List;

public record VectorStoreBenchmarkSideResult(
        String vectorStoreName,
        int caseCount,
        List<Integer> topKValues,
        List<RetrievalEvaluationSummaryResponse> summary,
        List<RetrievalEvaluationCaseResultResponse> cases,
        long totalSearchLatencyNanos,
        int searchCount,
        LatencyStats latency
) {
}
