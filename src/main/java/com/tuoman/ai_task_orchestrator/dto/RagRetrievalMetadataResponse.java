package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class RagRetrievalMetadataResponse {

    private Integer topK;

    private Integer returnedCount;

    private String embeddingProvider;

    private String embeddingModel;

    private String distanceMetric;
}
