package com.tuoman.ai_task_orchestrator.dto;

import com.fasterxml.jackson.annotation.JsonInclude;
import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
@JsonInclude(JsonInclude.Include.NON_NULL)
public class RagGenerationMetadataResponse {

    private String provider;

    private String model;

    private String llmProvider;

    private String llmModel;

    private Boolean skipped;

    private String reason;
}
