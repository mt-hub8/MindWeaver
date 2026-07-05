package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.config.ModelProviderProperties;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingProperties;
import com.tuoman.ai_task_orchestrator.embedding.MockEmbeddingClient;
import com.tuoman.ai_task_orchestrator.entity.ModelProviderConfigEntity;
import com.tuoman.ai_task_orchestrator.llm.LlmProperties;
import com.tuoman.ai_task_orchestrator.llm.MockLlmProvider;
import com.tuoman.ai_task_orchestrator.modelprovider.ModelProviderType;
import com.tuoman.ai_task_orchestrator.modelprovider.ResolvedModelProvider;
import com.tuoman.ai_task_orchestrator.repository.ModelProviderConfigRepository;
import com.tuoman.ai_task_orchestrator.security.ApiKeySecretService;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

@Service
@RequiredArgsConstructor
public class ModelProviderSelectionService {

    private final ModelProviderConfigRepository repository;

    private final ApiKeySecretService apiKeySecretService;

    private final ModelProviderProperties modelProviderProperties;

    private final LlmProperties llmProperties;

    private final EmbeddingProperties embeddingProperties;

    @Transactional(readOnly = true)
    public ResolvedModelProvider resolveDefaultLlm() {
        if (shouldUseDatabaseDefaults()) {
            return repository.findByDefaultLlmTrue()
                    .filter(ModelProviderConfigEntity::isEnabled)
                    .map(entity -> fromEntity(entity, true))
                    .orElseGet(this::fromPropertiesLlm);
        }
        return fromPropertiesLlm();
    }

    @Transactional(readOnly = true)
    public ResolvedModelProvider resolveDefaultEmbedding() {
        if (shouldUseDatabaseDefaults()) {
            return repository.findByDefaultEmbeddingTrue()
                    .filter(ModelProviderConfigEntity::isEnabled)
                    .map(entity -> fromEntity(entity, false))
                    .orElseGet(this::fromPropertiesEmbedding);
        }
        return fromPropertiesEmbedding();
    }

    private boolean shouldUseDatabaseDefaults() {
        return modelProviderProperties.isDatabaseOverridesEnabled();
    }

    private ResolvedModelProvider fromPropertiesLlm() {
        if (MockLlmProvider.PROVIDER.equalsIgnoreCase(llmProperties.getProvider())) {
            return ResolvedModelProvider.builder()
                    .configId(null)
                    .providerType(ModelProviderType.MOCK)
                    .displayName("默认 Mock（application.properties）")
                    .baseUrl(null)
                    .apiKey(null)
                    .llmModel(MockLlmProvider.DEFAULT_MODEL)
                    .embeddingModel(null)
                    .embeddingDimension(null)
                    .fromDatabase(false)
                    .build();
        }
        return ResolvedModelProvider.builder()
                .configId(null)
                .providerType(ModelProviderType.OLLAMA)
                .displayName("本地 Python Worker（application.properties）")
                .baseUrl(llmProperties.getBaseUrl())
                .apiKey(null)
                .llmModel(llmProperties.getModel())
                .embeddingModel(null)
                .embeddingDimension(null)
                .fromDatabase(false)
                .build();
    }

    private ResolvedModelProvider fromPropertiesEmbedding() {
        if (MockEmbeddingClient.PROVIDER.equalsIgnoreCase(embeddingProperties.getProvider())) {
            return ResolvedModelProvider.builder()
                    .configId(null)
                    .providerType(ModelProviderType.MOCK)
                    .displayName("默认 Mock（application.properties）")
                    .baseUrl(null)
                    .apiKey(null)
                    .llmModel(null)
                    .embeddingModel(MockEmbeddingClient.DEFAULT_MODEL)
                    .embeddingDimension(MockEmbeddingClient.DIMENSION)
                    .fromDatabase(false)
                    .build();
        }
        EmbeddingProperties.LocalWorker worker = embeddingProperties.getLocalWorker();
        return ResolvedModelProvider.builder()
                .configId(null)
                .providerType(ModelProviderType.OLLAMA)
                .displayName("本地 Python Worker（application.properties）")
                .baseUrl(worker.getBaseUrl())
                .apiKey(null)
                .llmModel(null)
                .embeddingModel(worker.getModel())
                .embeddingDimension(worker.getDimension())
                .fromDatabase(false)
                .build();
    }

    private ResolvedModelProvider fromEntity(ModelProviderConfigEntity entity, boolean forLlm) {
        String apiKey = entity.getApiKeyEncrypted() == null
                ? null
                : apiKeySecretService.decrypt(entity.getApiKeyEncrypted());
        return ResolvedModelProvider.builder()
                .configId(entity.getId())
                .providerType(entity.getProviderType())
                .displayName(entity.getDisplayName())
                .baseUrl(entity.getBaseUrl())
                .apiKey(apiKey)
                .llmModel(entity.getDefaultLlmModel())
                .embeddingModel(entity.getDefaultEmbeddingModel())
                .embeddingDimension(entity.getEmbeddingDimension())
                .fromDatabase(true)
                .build();
    }

    public void ensureEnabled(ResolvedModelProvider provider) {
        if (provider.getProviderType() == ModelProviderType.MOCK) {
            return;
        }
        if (provider.isFromDatabase() && provider.getConfigId() != null) {
            ModelProviderConfigEntity entity = repository.findById(provider.getConfigId())
                    .orElseThrow(BusinessException::modelProviderNotFound);
            if (!entity.isEnabled()) {
                throw BusinessException.modelProviderDisabled("默认模型供应商已禁用");
            }
        }
    }
}
