package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.dto.ModelProviderTestResultResponse;
import com.tuoman.ai_task_orchestrator.entity.ModelProviderConfigEntity;
import com.tuoman.ai_task_orchestrator.modelprovider.ModelProviderTestStatus;
import com.tuoman.ai_task_orchestrator.modelprovider.ModelProviderType;
import com.tuoman.ai_task_orchestrator.security.ApiKeySecretService;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.web.client.ResourceAccessException;
import org.springframework.web.client.RestClient;

import java.time.LocalDateTime;

@Service
@RequiredArgsConstructor
public class ModelProviderTestService {

    private final ApiKeySecretService apiKeySecretService;

    private final RestClient.Builder restClientBuilder;

    public ModelProviderTestResultResponse test(ModelProviderConfigEntity entity) {
        long started = System.currentTimeMillis();
        LocalDateTime testedAt = LocalDateTime.now();

        if (entity.getProviderType() == ModelProviderType.MOCK) {
            return new ModelProviderTestResultResponse(
                    ModelProviderTestStatus.SUCCESS,
                    "Mock 供应商连接正常（仅用于开发测试）",
                    entity.getProviderType(),
                    testedAt,
                    System.currentTimeMillis() - started
            );
        }

        if (entity.getProviderType() == ModelProviderType.OLLAMA) {
            return testOllama(entity, testedAt, started);
        }

        if (entity.getProviderType() == ModelProviderType.OPENAI_COMPATIBLE
                || entity.getProviderType() == ModelProviderType.CUSTOM_OPENAI_COMPATIBLE) {
            return testOpenAiCompatible(entity, testedAt, started);
        }

        return new ModelProviderTestResultResponse(
                ModelProviderTestStatus.FAILED,
                "不支持的供应商类型",
                entity.getProviderType(),
                testedAt,
                System.currentTimeMillis() - started
        );
    }

    private ModelProviderTestResultResponse testOllama(
            ModelProviderConfigEntity entity,
            LocalDateTime testedAt,
            long started
    ) {
        if (entity.getBaseUrl() == null || entity.getBaseUrl().isBlank()) {
            return failed(entity, testedAt, started, "缺少 Ollama Base URL");
        }
        try {
            RestClient client = restClientBuilder.baseUrl(normalizeBaseUrl(entity.getBaseUrl())).build();
            client.get().uri("/api/tags").retrieve().toBodilessEntity();
            return new ModelProviderTestResultResponse(
                    ModelProviderTestStatus.SUCCESS,
                    "Ollama 连接正常",
                    entity.getProviderType(),
                    testedAt,
                    System.currentTimeMillis() - started
            );
        } catch (ResourceAccessException exception) {
            return failed(entity, testedAt, started, "无法连接 Ollama：" + exception.getMessage());
        } catch (Exception exception) {
            return failed(entity, testedAt, started, "Ollama 测试失败：" + exception.getMessage());
        }
    }

    private ModelProviderTestResultResponse testOpenAiCompatible(
            ModelProviderConfigEntity entity,
            LocalDateTime testedAt,
            long started
    ) {
        if (entity.getBaseUrl() == null || entity.getBaseUrl().isBlank()) {
            return failed(entity, testedAt, started, "缺少 Base URL");
        }
        String apiKey = entity.getApiKeyEncrypted() == null
                ? null
                : apiKeySecretService.decrypt(entity.getApiKeyEncrypted());
        if (apiKey == null || apiKey.isBlank()) {
            return failed(entity, testedAt, started, "缺少 API Key");
        }
        try {
            RestClient client = restClientBuilder.baseUrl(normalizeBaseUrl(entity.getBaseUrl())).build();
            client.get()
                    .uri("/models")
                    .header("Authorization", "Bearer " + apiKey)
                    .retrieve()
                    .toBodilessEntity();
            return new ModelProviderTestResultResponse(
                    ModelProviderTestStatus.SUCCESS,
                    "OpenAI-compatible API 连接正常",
                    entity.getProviderType(),
                    testedAt,
                    System.currentTimeMillis() - started
            );
        } catch (ResourceAccessException exception) {
            return failed(entity, testedAt, started, "连接超时或网络不可达");
        } catch (Exception exception) {
            String message = exception.getMessage() == null ? "请求失败" : exception.getMessage();
            if (message.contains("401") || message.contains("Unauthorized")) {
                return failed(entity, testedAt, started, "API Key 无效或未授权");
            }
            return failed(entity, testedAt, started, "OpenAI-compatible 测试失败：" + message);
        }
    }

    private ModelProviderTestResultResponse failed(
            ModelProviderConfigEntity entity,
            LocalDateTime testedAt,
            long started,
            String message
    ) {
        return new ModelProviderTestResultResponse(
                ModelProviderTestStatus.FAILED,
                message,
                entity.getProviderType(),
                testedAt,
                System.currentTimeMillis() - started
        );
    }

    private String normalizeBaseUrl(String baseUrl) {
        return baseUrl.endsWith("/") ? baseUrl.substring(0, baseUrl.length() - 1) : baseUrl;
    }
}
