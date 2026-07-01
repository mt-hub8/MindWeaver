package com.tuoman.ai_task_orchestrator.dto;

import lombok.Getter;
import lombok.Setter;

@Getter
@Setter
public class RagAnswerRequest {

    private String query;

    private Long documentId;

    private Integer topK;

    private String requestedModel;
}
