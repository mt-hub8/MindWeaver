package com.tuoman.ai_task_orchestrator.dto;

import lombok.Getter;
import lombok.Setter;

import java.util.List;

@Getter
@Setter
public class RetrievalEvaluationCaseRequest {

    private String caseId;

    private String query;

    private List<Long> expectedChunkIds;
}
