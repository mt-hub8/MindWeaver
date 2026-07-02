package com.tuoman.ai_task_orchestrator.llm;

import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;

class MockLlmProviderTest {

    private final MockLlmProvider provider = new MockLlmProvider();

    @Test
    void generateShouldReturnAgentStyleAnswerForTaskPrompt() {
        LlmGenerateResult result = provider.generate(
                "system",
                "任务目标：总结项目\n知识库上下文：\n[1]\ncontent: demo",
                new LlmGenerateOptions()
        );

        assertThat(result.isSuccess()).isTrue();
        assertThat(result.getContent()).contains("任务结论");
        assertThat(result.getProvider()).isEqualTo(MockLlmProvider.PROVIDER);
    }

    @Test
    void generateShouldFailForFailureKeyword() {
        LlmGenerateResult result = provider.generate("system", "please fail now", new LlmGenerateOptions());
        assertThat(result.isSuccess()).isFalse();
    }
}
