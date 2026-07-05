package com.tuoman.ai_task_orchestrator.llm;

import com.tuoman.ai_task_orchestrator.modelprovider.ResolvedModelProvider;
import org.springframework.http.HttpHeaders;
import org.springframework.stereotype.Component;
import org.springframework.web.client.RestClient;

import java.time.Duration;

@Component
public class RestClientOpenAiCompatibleLlmHttpClient implements OpenAiCompatibleLlmHttpClient {

    private static final int TIMEOUT_MS = 30000;

    private final RestClient.Builder restClientBuilder;

    public RestClientOpenAiCompatibleLlmHttpClient(RestClient.Builder restClientBuilder) {
        this.restClientBuilder = restClientBuilder;
    }

    @Override
    public OpenAiChatResponse createChatCompletion(OpenAiChatRequest request, ResolvedModelProvider provider) {
        RestClient client = restClientBuilder
                .baseUrl(normalizeBaseUrl(provider.getBaseUrl()))
                .requestFactory(new org.springframework.http.client.JdkClientHttpRequestFactory(
                        java.net.http.HttpClient.newBuilder()
                                .connectTimeout(Duration.ofMillis(TIMEOUT_MS))
                                .build()
                ))
                .build();

        return client.post()
                .uri("/chat/completions")
                .header(HttpHeaders.AUTHORIZATION, "Bearer " + provider.getApiKey())
                .body(request)
                .retrieve()
                .body(OpenAiChatResponse.class);
    }

    private String normalizeBaseUrl(String baseUrl) {
        if (baseUrl == null || baseUrl.isBlank()) {
            return "https://api.openai.com/v1";
        }
        return baseUrl.endsWith("/") ? baseUrl.substring(0, baseUrl.length() - 1) : baseUrl;
    }
}
