package com.tuoman.ai_task_orchestrator.llm;

import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;

class LlmProviderConfigurationDefaultsTest {

    private final LlmProviderConfiguration configuration = new LlmProviderConfiguration();

    @Test
    void activeLlmProviderShouldDefaultToLocalPython() {
        LlmProperties properties = new LlmProperties();
        MockLlmProvider mock = new MockLlmProvider();
        LocalPythonLlmProvider localPython = new LocalPythonLlmProvider(
                properties,
                (request, llmProperties) -> new LocalPythonLlmResponse()
        );

        LlmProvider provider = configuration.propertyBasedLlmProvider(properties, mock, localPython);

        assertThat(provider).isSameAs(localPython);
        assertThat(provider.provider()).isEqualTo("local-python");
        assertThat(provider.defaultModel()).isEqualTo("qwen2.5:7b");
    }

    @Test
    void activeLlmProviderShouldUseMockOnlyWhenExplicitlyConfigured() {
        LlmProperties properties = new LlmProperties();
        properties.setProvider("mock");
        MockLlmProvider mock = new MockLlmProvider();
        LocalPythonLlmProvider localPython = new LocalPythonLlmProvider(
                properties,
                (request, llmProperties) -> new LocalPythonLlmResponse()
        );

        LlmProvider provider = configuration.propertyBasedLlmProvider(properties, mock, localPython);

        assertThat(provider).isSameAs(mock);
        assertThat(provider.provider()).isEqualTo("mock");
    }
}
