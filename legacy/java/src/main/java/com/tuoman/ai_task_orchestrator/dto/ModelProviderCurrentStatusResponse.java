package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class ModelProviderCurrentStatusResponse {

    private String activeProfile;

    private String llmProviderName;

    private Long llmProviderConfigId;

    private String llmModel;

    private String embeddingProviderName;

    private Long embeddingProviderConfigId;

    private String embeddingModel;

    private Integer embeddingDimension;

    private String runtimeModeDescription;

    private boolean embeddingDimensionMismatchWarning;

    private String embeddingDimensionMismatchMessage;
}
