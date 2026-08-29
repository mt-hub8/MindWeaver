package com.tuoman.ai_task_orchestrator.dto;

import com.tuoman.ai_task_orchestrator.modelprovider.ModelProviderTestStatus;
import com.tuoman.ai_task_orchestrator.modelprovider.ModelProviderType;
import lombok.AllArgsConstructor;
import lombok.Getter;

import java.time.LocalDateTime;

@Getter
@AllArgsConstructor
public class ModelProviderConfigResponse {

    private Long id;

    private ModelProviderType providerType;

    private String displayName;

    private String baseUrl;

    private String apiKeyMasked;

    private String defaultLlmModel;

    private String defaultEmbeddingModel;

    private String defaultRerankModel;

    private Integer embeddingDimension;

    private boolean enabled;

    private boolean defaultLlm;

    private boolean defaultEmbedding;

    private ModelProviderTestStatus lastTestStatus;

    private String lastTestMessage;

    private LocalDateTime lastTestedAt;

    private LocalDateTime createdAt;

    private LocalDateTime updatedAt;
}
