package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class DocumentEmbeddingResponse {

    private Long documentId;

    private String embeddingProvider;

    private String embeddingModel;

    private Integer vectorDimension;

    private String distanceMetric;

    private Integer embeddedChunkCount;
}
