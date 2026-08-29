package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.util.List;

@Getter
@AllArgsConstructor
public class RetrievalEvaluationResponse {

    private Long documentId;

    private Integer caseCount;

    private List<Integer> topKValues;

    private List<RetrievalEvaluationSummaryResponse> summary;

    private List<RetrievalEvaluationCaseResultResponse> cases;
}
