package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.util.List;

@Getter
@AllArgsConstructor
public class RetrievalEvaluationCaseResultResponse {

    private String caseId;

    private String query;

    private List<Long> expectedChunkIds;

    private List<RetrievedChunkEvaluationResponse> retrievedChunks;

    private List<RetrievalMetricAtKResponse> metrics;
}
