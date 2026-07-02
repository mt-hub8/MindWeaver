package com.tuoman.ai_task_orchestrator.llm;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.common.error.ErrorCode;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class LocalPythonLlmProviderTest {

    @Mock
    private LocalPythonLlmHttpClient httpClient;

    private LlmProperties properties;

    private LocalPythonLlmProvider provider;

    @BeforeEach
    void setUp() {
        properties = new LlmProperties();
        properties.setBaseUrl("http://127.0.0.1:8001");
        properties.setModel("mock-llm-template");
        properties.setTemperature(0.2);
        properties.setMaxTokens(1200);
        provider = new LocalPythonLlmProvider(properties, httpClient);
    }

    @Test
    void generateShouldMapWorkerResponse() {
        LocalPythonLlmResponse response = new LocalPythonLlmResponse();
        response.setProvider("local-python");
        response.setModel("mock-llm-template");
        response.setContent("generated answer");
        response.setLatencyMs(10L);
        LocalPythonLlmResponse.Usage usage = new LocalPythonLlmResponse.Usage();
        usage.setInputTokens(100);
        usage.setOutputTokens(50);
        response.setUsage(usage);
        when(httpClient.generate(any(), any())).thenReturn(response);

        LlmGenerateResult result = provider.generate("system", "user prompt with 知识库上下文", new LlmGenerateOptions());

        assertThat(result.isSuccess()).isTrue();
        assertThat(result.getContent()).isEqualTo("generated answer");
        assertThat(result.getProvider()).isEqualTo("local-python");
        assertThat(result.getInputTokens()).isEqualTo(100);
        assertThat(result.getOutputTokens()).isEqualTo(50);
    }

    @Test
    void generateShouldThrowWhenWorkerUnavailable() {
        when(httpClient.generate(any(), any()))
                .thenThrow(BusinessException.aiRuntimeUnavailable("connection refused"));

        assertThatThrownBy(() -> provider.generate("system", "user", new LlmGenerateOptions()))
                .isInstanceOf(BusinessException.class)
                .extracting("errorCode")
                .isEqualTo(ErrorCode.AI_RUNTIME_UNAVAILABLE);
    }
}
