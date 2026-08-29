package com.tuoman.ai_task_orchestrator.embedding;

import org.springframework.http.MediaType;
import org.springframework.stereotype.Component;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientException;
import org.springframework.web.client.RestClientResponseException;

import java.time.Duration;

/**
 * V2.5.2/V8.0 本地 Python embedding worker HTTP client。
 *
 * local-ai profile 下，文档 embedding 通过 Java -> Python Worker -> Ollama 生成。
 * 返回维度会在 EmbeddingCacheService 和 VectorNamespaceGuard 中继续校验。
 * 这里不做静默 fallback，因为错误 embedding 写入会污染后续检索和 reindex。
 */
@Component
public class RestClientLocalEmbeddingWorkerClient implements LocalEmbeddingWorkerClient {

    private final RestClient.Builder restClientBuilder;

    public RestClientLocalEmbeddingWorkerClient(RestClient.Builder restClientBuilder) {
        this.restClientBuilder = restClientBuilder;
    }

    @Override
    public LocalEmbeddingWorkerResponse createEmbeddings(
            LocalEmbeddingWorkerRequest request,
            EmbeddingProperties.LocalWorker properties
    ) {
        RestClient restClient = restClientBuilder
                .baseUrl(normalizeBaseUrl(properties.getBaseUrl()))
                .requestFactory(new org.springframework.http.client.JdkClientHttpRequestFactory(
                        java.net.http.HttpClient.newBuilder()
                                .connectTimeout(Duration.ofMillis(properties.getTimeoutMs()))
                                .build()
                ))
                .build();

        try {
            return restClient.post()
                    .uri("/embeddings")
                    .contentType(MediaType.APPLICATION_JSON)
                    .body(request)
                    .retrieve()
                    .body(LocalEmbeddingWorkerResponse.class);
        } catch (RestClientResponseException exception) {
            int status = exception.getStatusCode().value();
            if (status == 408 || status == 504) {
                throw new EmbeddingProviderException("Python embedding worker timeout: " + exception.getMessage());
            }
            if (status == 503) {
                throw new EmbeddingProviderException("Python embedding worker unavailable (Ollama may be down): "
                        + exception.getMessage());
            }
            throw new EmbeddingProviderException("Python embedding worker bad response: " + exception.getMessage());
        } catch (RestClientException exception) {
            throw new EmbeddingProviderException(
                    "Python embedding worker unavailable: " + exception.getMessage(),
                    exception
            );
        }
    }

    private String normalizeBaseUrl(String baseUrl) {
        if (baseUrl == null || baseUrl.isBlank()) {
            return "http://127.0.0.1:8001";
        }
        return baseUrl.endsWith("/") ? baseUrl.substring(0, baseUrl.length() - 1) : baseUrl;
    }
}
