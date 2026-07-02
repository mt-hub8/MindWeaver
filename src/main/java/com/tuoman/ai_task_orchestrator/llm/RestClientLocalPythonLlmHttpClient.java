package com.tuoman.ai_task_orchestrator.llm;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import org.springframework.http.MediaType;
import org.springframework.stereotype.Component;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientException;
import org.springframework.web.client.RestClientResponseException;

import java.time.Duration;

@Component
public class RestClientLocalPythonLlmHttpClient implements LocalPythonLlmHttpClient {

    private final RestClient.Builder restClientBuilder;

    public RestClientLocalPythonLlmHttpClient(RestClient.Builder restClientBuilder) {
        this.restClientBuilder = restClientBuilder;
    }

    @Override
    public LocalPythonLlmResponse generate(LocalPythonLlmRequest request, LlmProperties properties) {
        RestClient client = restClientBuilder
                .baseUrl(properties.getBaseUrl())
                .build();
        try {
            return client.post()
                    .uri("/generate")
                    .contentType(MediaType.APPLICATION_JSON)
                    .body(request)
                    .retrieve()
                    .body(LocalPythonLlmResponse.class);
        } catch (RestClientResponseException exception) {
            if (exception.getStatusCode().value() == 408 || exception.getStatusCode().value() == 504) {
                throw BusinessException.aiRuntimeTimeout("Python LLM worker timeout: " + exception.getMessage());
            }
            throw BusinessException.aiRuntimeUnavailable("Python LLM worker error: " + exception.getMessage());
        } catch (RestClientException exception) {
            throw BusinessException.aiRuntimeUnavailable(
                    "Python LLM worker unavailable: " + exception.getMessage()
            );
        }
    }
}
