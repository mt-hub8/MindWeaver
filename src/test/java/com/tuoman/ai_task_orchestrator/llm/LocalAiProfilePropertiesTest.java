package com.tuoman.ai_task_orchestrator.llm;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.common.error.ErrorCode;
import org.junit.jupiter.api.Test;

import java.io.IOException;
import java.nio.file.Path;

import java.util.Properties;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

class LocalAiProfilePropertiesTest {

    @Test
    void defaultApplicationPropertiesShouldUseLocalWorkerEmbedding() throws IOException {
        Properties properties = loadProperties(Path.of("src/main/resources/application.properties"));
        assertThat(properties.getProperty("app.embedding.provider")).isEqualTo("local-worker");
        assertThat(properties.getProperty("app.embedding.local-worker.base-url")).isEqualTo("http://127.0.0.1:8001");
        assertThat(properties.getProperty("app.embedding.local-worker.model")).isEqualTo("qwen3-embedding:0.6b");
        assertThat(properties.getProperty("app.embedding.local-worker.dimension")).isEqualTo("1024");
        assertThat(properties.getProperty("app.llm.provider")).isEqualTo("local-python");
        assertThat(properties.getProperty("app.llm.model")).isEqualTo("qwen2.5:7b");
    }

    @Test
    void localAiProfileShouldConfigureOllamaModels() throws IOException {
        Properties properties = loadProperties(Path.of("src/main/resources/application-local-ai.properties"));
        assertThat(properties.getProperty("app.embedding.provider")).isEqualTo("local-worker");
        assertThat(properties.getProperty("app.embedding.local-worker.model")).isEqualTo("qwen3-embedding:0.6b");
        assertThat(properties.getProperty("app.embedding.local-worker.dimension")).isEqualTo("1024");
        assertThat(properties.getProperty("app.llm.provider")).isEqualTo("local-python");
        assertThat(properties.getProperty("app.llm.model")).isEqualTo("qwen2.5:7b");
        assertThat(properties.getProperty("app.llm.base-url")).contains("8001");
    }

    @Test
    void localPythonLlmRequestShouldMapFields() {
        LlmProperties properties = new LlmProperties();
        properties.setModel("qwen2.5:7b");
        properties.setTemperature(0.2);
        properties.setMaxTokens(1200);

        LocalPythonLlmHttpClient httpClient = mock(LocalPythonLlmHttpClient.class);
        LocalPythonLlmResponse response = new LocalPythonLlmResponse();
        response.setProvider("local-ollama");
        response.setModel("qwen2.5:7b");
        response.setContent("报告");
        response.setLatencyMs(100L);
        when(httpClient.generate(any(), any())).thenAnswer(invocation -> {
            LocalPythonLlmRequest request = invocation.getArgument(0);
            assertThat(request.getUserPrompt()).isEqualTo("任务目标");
            assertThat(request.getSystemPrompt()).isEqualTo("系统");
            assertThat(request.getModel()).isEqualTo("qwen2.5:7b");
            assertThat(request.getTemperature()).isEqualTo(0.2);
            assertThat(request.getMaxTokens()).isEqualTo(1200);
            return response;
        });

        LocalPythonLlmProvider provider = new LocalPythonLlmProvider(properties, httpClient);
        LlmGenerateResult result = provider.generate("系统", "任务目标", new LlmGenerateOptions());

        assertThat(result.isSuccess()).isTrue();
        assertThat(result.getProvider()).isEqualTo("local-ollama");
        assertThat(result.getModel()).isEqualTo("qwen2.5:7b");
        assertThat(result.getLatencyMs()).isEqualTo(100L);
    }

    @Test
    void localPythonLlmShouldMapTimeoutToAiRuntimeTimeout() {
        LlmProperties properties = new LlmProperties();
        LocalPythonLlmHttpClient httpClient = mock(LocalPythonLlmHttpClient.class);
        when(httpClient.generate(any(), any())).thenThrow(BusinessException.aiRuntimeTimeout("timeout"));

        LocalPythonLlmProvider provider = new LocalPythonLlmProvider(properties, httpClient);
        assertThatThrownBy(() -> provider.generate("s", "u", new LlmGenerateOptions()))
                .isInstanceOf(BusinessException.class)
                .extracting("errorCode")
                .isEqualTo(ErrorCode.AI_RUNTIME_TIMEOUT);
    }

    @Test
    void localPythonLlmShouldMapBadResponseToAiRuntimeBadResponse() {
        LlmProperties properties = new LlmProperties();
        LocalPythonLlmHttpClient httpClient = mock(LocalPythonLlmHttpClient.class);
        when(httpClient.generate(any(), any())).thenThrow(BusinessException.aiRuntimeBadResponse("bad body"));

        LocalPythonLlmProvider provider = new LocalPythonLlmProvider(properties, httpClient);
        assertThatThrownBy(() -> provider.generate("s", "u", new LlmGenerateOptions()))
                .isInstanceOf(BusinessException.class)
                .extracting("errorCode")
                .isEqualTo(ErrorCode.AI_RUNTIME_BAD_RESPONSE);
    }

    private Properties loadProperties(Path propertiesPath) throws IOException {
        Properties properties = new Properties();
        try (var inputStream = java.nio.file.Files.newInputStream(propertiesPath)) {
            properties.load(inputStream);
        }
        return properties;
    }
}
