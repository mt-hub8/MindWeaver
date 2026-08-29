package com.tuoman.ai_task_orchestrator.dto;

import lombok.Getter;
import lombok.Setter;

import java.util.List;

@Getter
@Setter
public class RetrievalEvaluationRequest {

    private Long documentId;

    private List<Integer> topKValues;

    private List<RetrievalEvaluationCaseRequest> cases;
}
