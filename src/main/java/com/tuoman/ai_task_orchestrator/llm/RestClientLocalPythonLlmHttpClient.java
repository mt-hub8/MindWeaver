package com.tuoman.ai_task_orchestrator.llm;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import org.springframework.http.MediaType;
import org.springframework.stereotype.Component;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientException;
import org.springframework.web.client.RestClientResponseException;

import java.time.Duration;

/**
 * 本地 Python LLM worker HTTP client。
 *
 * Java 后端只调用 /generate；Python worker 负责再去访问 Ollama 或其他本地 runtime。
 * 这样本地 AI runtime 细节不会扩散到 RAG/Agent 业务服务。
 */
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
            int status = exception.getStatusCode().value();
            if (status == 408 || status == 504) {
                throw BusinessException.aiRuntimeTimeout("Python LLM worker timeout: " + exception.getMessage());
            }
            if (status == 503) {
                throw BusinessException.aiRuntimeUnavailable("Python LLM worker unavailable: " + exception.getMessage());
            }
            throw BusinessException.aiRuntimeBadResponse("Python LLM worker bad response: " + exception.getMessage());
        } catch (RestClientException exception) {
            throw BusinessException.aiRuntimeUnavailable(
                    "Python LLM worker unavailable: " + exception.getMessage()
            );
        }
    }
}
