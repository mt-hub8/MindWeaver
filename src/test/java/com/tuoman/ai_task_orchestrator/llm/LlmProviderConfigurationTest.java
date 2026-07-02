package com.tuoman.ai_task_orchestrator.llm;

import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.test.context.TestPropertySource;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest
@TestPropertySource(properties = {
        "app.llm.provider=mock",
        "app.embedding.provider=mock"
})
class LlmProviderConfigurationTest {

    @Autowired
    private LlmProvider activeLlmProvider;

    @Autowired
    private LlmClient llmClient;

    @Test
    void defaultProfileShouldUseMockLlmProvider() {
        assertThat(activeLlmProvider.provider()).isEqualTo(MockLlmProvider.PROVIDER);
        assertThat(activeLlmProvider.defaultModel()).isEqualTo(MockLlmProvider.DEFAULT_MODEL);
    }

    @Test
    void delegatingLlmClientShouldReturnSuccessForValidPrompt() {
        LlmRequest request = new LlmRequest();
        request.setPrompt("hello agent task");
        LlmResponse response = llmClient.generate(request);
        assertThat(response.isSuccess()).isTrue();
        assertThat(response.getProvider()).isEqualTo(MockLlmProvider.PROVIDER);
        assertThat(response.getContent()).isNotBlank();
    }
}
