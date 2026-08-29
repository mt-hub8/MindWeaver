package com.tuoman.ai_task_orchestrator.dto;

import com.tuoman.ai_task_orchestrator.modelprovider.ModelProviderType;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.NotNull;
import jakarta.validation.constraints.Size;
import lombok.Getter;
import lombok.Setter;

@Getter
@Setter
public class ModelProviderConfigRequest {

    @NotNull
    private ModelProviderType providerType;

    @NotBlank
    @Size(max = 128)
    private String displayName;

    @Size(max = 512)
    private String baseUrl;

    /**
     * Plain API key from client. Never logged. Empty on update means keep existing key.
     */
    @Size(max = 512)
    private String apiKey;

    @Size(max = 128)
    private String defaultLlmModel;

    @Size(max = 128)
    private String defaultEmbeddingModel;

    @Size(max = 128)
    private String defaultRerankModel;

    private Integer embeddingDimension;

    private Boolean enabled;
}
