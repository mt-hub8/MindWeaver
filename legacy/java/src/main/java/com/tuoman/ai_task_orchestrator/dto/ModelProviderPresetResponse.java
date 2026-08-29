package com.tuoman.ai_task_orchestrator.dto;

import com.tuoman.ai_task_orchestrator.modelprovider.ModelProviderType;
import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class ModelProviderPresetResponse {

    private String presetId;

    private ModelProviderType providerType;

    private String displayName;

    private String baseUrl;

    private String defaultLlmModel;

    private String defaultEmbeddingModel;

    private Integer embeddingDimension;

    private String description;
}
