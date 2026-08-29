package com.tuoman.ai_task_orchestrator.llm;

import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;

class MockLlmClientTest {

    private final MockLlmClient mockLlmClient = new MockLlmClient();

    @Test
    void shouldReturnDeterministicRagAnswerWithCitationMarkers() {
        LlmRequest request = new LlmRequest();
        request.setPrompt("""
                你是一个基于文档回答问题的助手。

                上下文：
                [1]
                documentId: 1
                chunkId: 10
                content: first

                [2]
                documentId: 1
                chunkId: 11
                content: second

                用户问题：
                Why cache key includes provider?
                """);
        request.setModel("mock-llm");

        LlmResponse response = mockLlmClient.generate(request);

        assertThat(response.isSuccess()).isTrue();
        assertThat(response.getProvider()).isEqualTo("mock");
        assertThat(response.getModel()).isEqualTo("mock-llm");
        assertThat(response.getContent()).isEqualTo("根据检索到的上下文，问题与以下来源相关：[1] [2]");
    }

    @Test
    void shouldReturnFailureForExplicitFailurePrompt() {
        LlmRequest request = new LlmRequest();
        request.setPrompt("please fail");

        LlmResponse response = mockLlmClient.generate(request);

        assertThat(response.isSuccess()).isFalse();
        assertThat(response.getContent()).isNull();
    }
}
