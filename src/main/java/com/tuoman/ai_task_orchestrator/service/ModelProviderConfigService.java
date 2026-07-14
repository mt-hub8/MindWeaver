package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.config.ModelProviderProperties;
import com.tuoman.ai_task_orchestrator.dto.ModelProviderConfigRequest;
import com.tuoman.ai_task_orchestrator.dto.ModelProviderConfigResponse;
import com.tuoman.ai_task_orchestrator.dto.ModelProviderCurrentStatusResponse;
import com.tuoman.ai_task_orchestrator.dto.ModelProviderPresetResponse;
import com.tuoman.ai_task_orchestrator.dto.ModelProviderTestResultResponse;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingProperties;
import com.tuoman.ai_task_orchestrator.embedding.MockEmbeddingClient;
import com.tuoman.ai_task_orchestrator.entity.ModelProviderConfigEntity;
import com.tuoman.ai_task_orchestrator.llm.LlmProperties;
import com.tuoman.ai_task_orchestrator.llm.MockLlmProvider;
import com.tuoman.ai_task_orchestrator.modelprovider.ModelProviderPresetCatalog;
import com.tuoman.ai_task_orchestrator.modelprovider.ModelProviderType;
import com.tuoman.ai_task_orchestrator.modelprovider.ResolvedModelProvider;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkEmbeddingRepository;
import com.tuoman.ai_task_orchestrator.repository.ModelProviderConfigRepository;
import com.tuoman.ai_task_orchestrator.security.ApiKeySecretService;
import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import lombok.RequiredArgsConstructor;
import org.springframework.core.env.Environment;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.Arrays;
import java.util.List;
import java.util.stream.Collectors;

/**
 * V10 模型供应商配置服务。
 *
 * 管理 mock、Ollama/local-ai、OpenAI-compatible 和自定义 OpenAI-compatible 配置，
 * 同时提供当前默认模型状态和 embedding dimension 风险提示。
 */
@Service
@RequiredArgsConstructor
public class ModelProviderConfigService {

    private final ModelProviderConfigRepository repository;

    private final ApiKeySecretService apiKeySecretService;

    private final ModelProviderTestService testService;

    private final ModelProviderSelectionService selectionService;

    private final DocumentChunkEmbeddingRepository embeddingRepository;

    private final Environment environment;

    private final LlmProperties llmProperties;

    private final EmbeddingProperties embeddingProperties;

    @Transactional(readOnly = true)
    public List<ModelProviderConfigResponse> listAll() {
        return repository.findAllByOrderByIdAsc().stream()
                .map(this::toResponse)
                .toList();
    }

    @Transactional(readOnly = true)
    public ModelProviderConfigResponse getById(Long id) {
        return toResponse(findEntity(id));
    }

    @Transactional(readOnly = true)
    public List<ModelProviderPresetResponse> listPresets() {
        return ModelProviderPresetCatalog.all();
    }

    @Transactional(readOnly = true)
    public ModelProviderCurrentStatusResponse currentStatus() {
        // embedding dimension 与已索引向量不一致时提示 reindex。
        // 模型切换本身不应自动改写旧向量，否则会造成新旧维度混用。
        ResolvedModelProvider llm = selectionService.resolveDefaultLlm();
        ResolvedModelProvider embedding = selectionService.resolveDefaultEmbedding();

        boolean dimensionWarning = false;
        String dimensionMessage = null;
        if (embedding.getEmbeddingDimension() != null) {
            List<Integer> existing = embeddingRepository.findDistinctVectorDimensions();
            if (!existing.isEmpty() && existing.stream().anyMatch(d -> !d.equals(embedding.getEmbeddingDimension()))) {
                dimensionWarning = true;
                dimensionMessage = "切换 Embedding 模型后，已有文档可能需要重新索引（当前索引维度与配置不一致）。";
            }
        }

        String profile = environment.getActiveProfiles().length == 0
                ? "default"
                : Arrays.stream(environment.getActiveProfiles()).collect(Collectors.joining(","));

        String modeDescription;
        if (MockLlmProvider.PROVIDER.equalsIgnoreCase(llmProperties.getProvider())
                && MockEmbeddingClient.PROVIDER.equalsIgnoreCase(embeddingProperties.getProvider())
                && !llm.isFromDatabase()) {
            modeDescription = "当前使用默认 mock，仅用于开发测试";
        } else if (llm.getProviderType() == ModelProviderType.OLLAMA
                || embedding.getProviderType() == ModelProviderType.OLLAMA) {
            modeDescription = "当前使用本地 Ollama 模型链路";
        } else {
            modeDescription = "当前使用已配置的模型供应商";
        }

        return new ModelProviderCurrentStatusResponse(
                profile,
                llm.getDisplayName(),
                llm.getConfigId(),
                llm.getLlmModel(),
                embedding.getDisplayName(),
                embedding.getConfigId(),
                embedding.getEmbeddingModel(),
                embedding.getEmbeddingDimension(),
                modeDescription,
                dimensionWarning,
                dimensionMessage
        );
    }

    @Transactional
    public ModelProviderConfigResponse create(ModelProviderConfigRequest request) {
        validateRequest(request, true);
        if (repository.findByDisplayName(request.getDisplayName().trim()).isPresent()) {
            throw BusinessException.modelProviderInvalid("显示名称已存在");
        }
        ModelProviderConfigEntity entity = new ModelProviderConfigEntity();
        applyRequest(entity, request, true);
        if (entity.isEnabled() && request.getProviderType() == ModelProviderType.MOCK) {
            // mock provider is informational only
        }
        return toResponse(repository.save(entity));
    }

    @Transactional
    public ModelProviderConfigResponse update(Long id, ModelProviderConfigRequest request) {
        validateRequest(request, false);
        ModelProviderConfigEntity entity = findEntity(id);
        if (!entity.getDisplayName().equals(request.getDisplayName().trim())
                && repository.findByDisplayName(request.getDisplayName().trim()).isPresent()) {
            throw BusinessException.modelProviderInvalid("显示名称已存在");
        }
        applyRequest(entity, request, false);
        return toResponse(repository.save(entity));
    }

    @Transactional
    public ModelProviderConfigResponse enable(Long id) {
        ModelProviderConfigEntity entity = findEntity(id);
        entity.setEnabled(true);
        return toResponse(repository.save(entity));
    }

    @Transactional
    public ModelProviderConfigResponse disable(Long id) {
        ModelProviderConfigEntity entity = findEntity(id);
        if (entity.isDefaultLlm() || entity.isDefaultEmbedding()) {
            throw BusinessException.modelProviderDisabled("请先取消默认问答模型或默认向量模型，再禁用该供应商");
        }
        if (entity.getProviderType() == ModelProviderType.MOCK) {
            throw BusinessException.modelProviderDisabled("内置 Mock 供应商不允许禁用");
        }
        entity.setEnabled(false);
        return toResponse(repository.save(entity));
    }

    @Transactional
    public ModelProviderConfigResponse setDefaultLlm(Long id) {
        ModelProviderConfigEntity entity = findEntity(id);
        if (!entity.isEnabled()) {
            throw BusinessException.modelProviderDisabled("已禁用的供应商不能设为默认问答模型");
        }
        repository.clearDefaultLlm();
        entity.setDefaultLlm(true);
        return toResponse(repository.save(entity));
    }

    @Transactional
    public ModelProviderConfigResponse setDefaultEmbedding(Long id) {
        ModelProviderConfigEntity entity = findEntity(id);
        if (!entity.isEnabled()) {
            throw BusinessException.modelProviderDisabled("已禁用的供应商不能设为默认向量模型");
        }
        repository.clearDefaultEmbedding();
        entity.setDefaultEmbedding(true);
        return toResponse(repository.save(entity));
    }

    @Transactional
    public ModelProviderTestResultResponse testConnection(Long id) {
        // connection test 只更新 lastTestStatus/lastTestMessage。
        // 它不能自动设为默认 provider，也不能因为 embedding 模型变化自动 reindex。
        ModelProviderConfigEntity entity = findEntity(id);
        ModelProviderTestResultResponse result = testService.test(entity);
        entity.setLastTestStatus(result.getStatus());
        entity.setLastTestMessage(result.getMessage());
        entity.setLastTestedAt(result.getTestedAt());
        repository.save(entity);
        return result;
    }

    private ModelProviderConfigEntity findEntity(Long id) {
        return repository.findById(id).orElseThrow(BusinessException::modelProviderNotFound);
    }

    private void validateRequest(ModelProviderConfigRequest request, boolean creating) {
        if (request.getProviderType() == null) {
            throw BusinessException.modelProviderInvalid("providerType 不能为空");
        }
        if (request.getDisplayName() == null || request.getDisplayName().isBlank()) {
            throw BusinessException.modelProviderInvalid("displayName 不能为空");
        }
        if (request.getProviderType() == ModelProviderType.OLLAMA) {
            if (request.getBaseUrl() == null || request.getBaseUrl().isBlank()) {
                throw BusinessException.modelProviderBaseUrlRequired();
            }
        }
        if (request.getProviderType() == ModelProviderType.OPENAI_COMPATIBLE
                || request.getProviderType() == ModelProviderType.CUSTOM_OPENAI_COMPATIBLE) {
            if (creating && (request.getBaseUrl() == null || request.getBaseUrl().isBlank())) {
                throw BusinessException.modelProviderBaseUrlRequired();
            }
            if (creating && (request.getApiKey() == null || request.getApiKey().isBlank())) {
                throw BusinessException.modelProviderApiKeyRequired();
            }
        }
    }

    private void applyRequest(ModelProviderConfigEntity entity, ModelProviderConfigRequest request, boolean creating) {
        entity.setProviderType(request.getProviderType());
        entity.setDisplayName(request.getDisplayName().trim());
        entity.setBaseUrl(trimToNull(request.getBaseUrl()));
        entity.setDefaultLlmModel(trimToNull(request.getDefaultLlmModel()));
        entity.setDefaultEmbeddingModel(trimToNull(request.getDefaultEmbeddingModel()));
        entity.setDefaultRerankModel(trimToNull(request.getDefaultRerankModel()));
        entity.setEmbeddingDimension(request.getEmbeddingDimension());
        if (request.getEnabled() != null) {
            entity.setEnabled(request.getEnabled());
        } else if (creating) {
            entity.setEnabled(true);
        }

        if (request.getApiKey() != null && !request.getApiKey().isBlank()) {
            String encrypted = apiKeySecretService.encrypt(request.getApiKey().trim());
            entity.setApiKeyEncrypted(encrypted);
            entity.setApiKeyMasked(apiKeySecretService.mask(request.getApiKey().trim()));
        } else if (creating && requiresApiKey(request.getProviderType())) {
            throw BusinessException.modelProviderApiKeyRequired();
        }
    }

    private boolean requiresApiKey(ModelProviderType type) {
        return type == ModelProviderType.OPENAI_COMPATIBLE
                || type == ModelProviderType.CUSTOM_OPENAI_COMPATIBLE;
    }

    private String trimToNull(String value) {
        if (value == null || value.isBlank()) {
            return null;
        }
        return value.trim();
    }

    private ModelProviderConfigResponse toResponse(ModelProviderConfigEntity entity) {
        return new ModelProviderConfigResponse(
                entity.getId(),
                entity.getProviderType(),
                entity.getDisplayName(),
                entity.getBaseUrl(),
                entity.getApiKeyMasked(),
                entity.getDefaultLlmModel(),
                entity.getDefaultEmbeddingModel(),
                entity.getDefaultRerankModel(),
                entity.getEmbeddingDimension(),
                entity.isEnabled(),
                entity.isDefaultLlm(),
                entity.isDefaultEmbedding(),
                entity.getLastTestStatus(),
                entity.getLastTestMessage(),
                entity.getLastTestedAt(),
                entity.getCreatedAt(),
                entity.getUpdatedAt()
        );
    }
}
