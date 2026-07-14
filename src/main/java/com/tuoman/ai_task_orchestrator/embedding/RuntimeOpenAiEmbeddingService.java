package com.tuoman.ai_task_orchestrator.embedding;

import com.tuoman.ai_task_orchestrator.modelprovider.ResolvedModelProvider;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;

import java.util.Comparator;
import java.util.List;

@Service
@RequiredArgsConstructor
/**
 * 运行时 OpenAI-compatible embedding 调用服务。
 *
 * 用于数据库模型供应商配置解析后的动态 embedding 调用，不依赖 application.properties
 * 中的静态 provider。它仍然只生成 embedding，不负责写 vector，也不负责自动 reindex。
 */
public class RuntimeOpenAiEmbeddingService {

    public static final String PROVIDER = "openai-compatible";

    private final OpenAiEmbeddingHttpClient httpClient;

    public EmbeddingResponse embed(EmbeddingRequest request, ResolvedModelProvider config) {
        return embedBatch(List.of(request == null ? new EmbeddingRequest() : request), config).getFirst();
    }

    public List<EmbeddingResponse> embedBatch(List<EmbeddingRequest> requests, ResolvedModelProvider config) {
        if (requests == null || requests.isEmpty()) {
            return List.of();
        }
        // 动态 provider 配置必须在调用前校验完整性。
        // API Key 只在内存中用于本次请求，不进入响应或日志。
        validateConfig(config);
        List<String> inputs = requests.stream()
                .map(r -> r == null || r.getText() == null ? "" : r.getText())
                .toList();

        EmbeddingProperties.OpenAi properties = toOpenAiProperties(config);
        OpenAiEmbeddingRequest openAiRequest = new OpenAiEmbeddingRequest(resolveModel(requests.getFirst(), config), inputs);
        OpenAiEmbeddingResponse response = httpClient.createEmbeddings(openAiRequest, properties);
        validateResponse(response, inputs.size(), properties);

        return response.getData().stream()
                .sorted(Comparator.comparing(OpenAiEmbeddingResponse.EmbeddingData::getIndex))
                .map(data -> new EmbeddingResponse(
                        PROVIDER,
                        resolveModel(requests.getFirst(), config),
                        data.getEmbedding().size(),
                        OpenAiCompatibleEmbeddingProvider.DISTANCE_METRIC,
                        data.getEmbedding()
                ))
                .toList();
    }

    private EmbeddingProperties.OpenAi toOpenAiProperties(ResolvedModelProvider config) {
        EmbeddingProperties.OpenAi openAi = new EmbeddingProperties.OpenAi();
        openAi.setBaseUrl(config.getBaseUrl());
        openAi.setApiKey(config.getApiKey());
        openAi.setModel(config.getEmbeddingModel());
        openAi.setDimension(config.getEmbeddingDimension());
        openAi.setTimeoutMs(10000);
        return openAi;
    }

    private void validateConfig(ResolvedModelProvider config) {
        if (config.getBaseUrl() == null || config.getBaseUrl().isBlank()) {
            throw new EmbeddingProviderException("OpenAI-compatible embedding base URL must not be blank");
        }
        if (config.getApiKey() == null || config.getApiKey().isBlank()) {
            throw new EmbeddingProviderException("OpenAI-compatible embedding api key must not be blank");
        }
        if (config.getEmbeddingModel() == null || config.getEmbeddingModel().isBlank()) {
            throw new EmbeddingProviderException("OpenAI-compatible embedding model must not be blank");
        }
    }

    private String resolveModel(EmbeddingRequest request, ResolvedModelProvider config) {
        if (request != null && request.getModel() != null && !request.getModel().isBlank()) {
            return request.getModel();
        }
        return config.getEmbeddingModel();
    }

    private void validateResponse(OpenAiEmbeddingResponse response, int expectedCount, EmbeddingProperties.OpenAi properties) {
        // 响应数量必须和输入数量一致，避免 chunk 与 embedding 错位。
        // 维度一致性会在后续 cache/vector 写入边界继续校验。
        if (response == null || response.getData() == null || response.getData().isEmpty()) {
            throw new EmbeddingProviderException("OpenAI-compatible embedding response data must not be empty");
        }
        if (response.getData().size() != expectedCount) {
            throw new EmbeddingProviderException("OpenAI-compatible embedding response size mismatch");
        }
    }
}
