package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.embedding.MockEmbeddingClient;
import com.tuoman.ai_task_orchestrator.llm.LlmProvider;
import com.tuoman.ai_task_orchestrator.llm.MockLlmProvider;
import com.tuoman.ai_task_orchestrator.modelprovider.ModelProviderType;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest
class ModelProviderSelectionServiceTest {

    @Autowired
    private ModelProviderSelectionService selectionService;

    @Autowired
    private LlmProvider llmProvider;

    @Test
    void defaultProfileShouldStillUseMockFromProperties() {
        assertThat(selectionService.resolveDefaultLlm().getProviderType()).isEqualTo(ModelProviderType.MOCK);
        assertThat(selectionService.resolveDefaultEmbedding().getProviderType()).isEqualTo(ModelProviderType.MOCK);
        assertThat(llmProvider.provider()).isEqualToIgnoringCase("mock");
        assertThat(llmProvider.defaultModel()).isEqualTo(MockLlmProvider.DEFAULT_MODEL);
    }

    @Test
    void resolvedEmbeddingShouldExposeMockDimension() {
        assertThat(selectionService.resolveDefaultEmbedding().getEmbeddingDimension())
                .isEqualTo(MockEmbeddingClient.DIMENSION);
    }
}
