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

    private Long latencyMs;

    private Integer inputTokens;

    private Integer outputTokens;

    public RagGenerationMetadataResponse(
            String provider,
            String model,
            String llmProvider,
            String llmModel,
            Boolean skipped,
            String reason
    ) {
        this(provider, model, llmProvider, llmModel, skipped, reason, null, null, null);
    }
}
