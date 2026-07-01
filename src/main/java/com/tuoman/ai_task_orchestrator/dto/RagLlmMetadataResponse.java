package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class RagLlmMetadataResponse {

    private String provider;

    private String model;

    private Integer promptTokenCount;

    private Integer completionTokenCount;

    private Integer totalTokenCount;

    private Long latencyMs;
}
