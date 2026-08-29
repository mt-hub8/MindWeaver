package com.tuoman.ai_task_orchestrator.embedding;

import org.springframework.http.HttpHeaders;
import org.springframework.stereotype.Component;
import org.springframework.web.client.RestClient;

import java.time.Duration;

@Component
public class RestClientOpenAiEmbeddingHttpClient implements OpenAiEmbeddingHttpClient {

    private final RestClient.Builder restClientBuilder;

    public RestClientOpenAiEmbeddingHttpClient(RestClient.Builder restClientBuilder) {
        this.restClientBuilder = restClientBuilder;
    }

    @Override
    public OpenAiEmbeddingResponse createEmbeddings(
            OpenAiEmbeddingRequest request,
            EmbeddingProperties.OpenAi properties
    ) {
        RestClient restClient = restClientBuilder
                .baseUrl(normalizeBaseUrl(properties.getBaseUrl()))
                .requestFactory(new org.springframework.http.client.JdkClientHttpRequestFactory(
                        java.net.http.HttpClient.newBuilder()
                                .connectTimeout(Duration.ofMillis(properties.getTimeoutMs()))
                                .build()
                ))
                .build();

        return restClient.post()
                .uri("/embeddings")
                .header(HttpHeaders.AUTHORIZATION, "Bearer " + properties.getApiKey())
                .body(request)
                .retrieve()
                .body(OpenAiEmbeddingResponse.class);
    }

    private String normalizeBaseUrl(String baseUrl) {
        if (baseUrl == null || baseUrl.isBlank()) {
            return "https://api.openai.com/v1";
        }
        return baseUrl.endsWith("/") ? baseUrl.substring(0, baseUrl.length() - 1) : baseUrl;
    }
}
