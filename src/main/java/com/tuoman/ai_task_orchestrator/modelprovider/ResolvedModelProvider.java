package com.tuoman.ai_task_orchestrator.modelprovider;

import lombok.Builder;
import lombok.Getter;

@Getter
@Builder
public class ResolvedModelProvider {

    private final Long configId;

    private final ModelProviderType providerType;

    private final String displayName;

    private final String baseUrl;

    private final String apiKey;

    private final String llmModel;

    private final String embeddingModel;

    private final Integer embeddingDimension;

    private final boolean fromDatabase;
}
