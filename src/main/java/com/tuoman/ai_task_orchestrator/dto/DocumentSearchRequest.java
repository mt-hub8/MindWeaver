package com.tuoman.ai_task_orchestrator.dto;

import lombok.Getter;
import lombok.Setter;

@Getter
@Setter
public class DocumentSearchRequest {

    private String query;

    private Integer topK;

    private String embeddingProvider;

    private String embeddingModel;

    private Long documentId;
}
