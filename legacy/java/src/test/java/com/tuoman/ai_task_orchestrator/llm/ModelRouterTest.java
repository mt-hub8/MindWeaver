package com.tuoman.ai_task_orchestrator.llm;

import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;

class ModelRouterTest {

    private final ModelRouter modelRouter = new ModelRouter();

    @Test
    void shouldReturnDefaultModelWhenRequestedModelIsNull() {
        assertThat(modelRouter.route(null)).isEqualTo("mock-llm");
    }

    @Test
    void shouldReturnMockFastWhenRequestedModelIsMockFast() {
        assertThat(modelRouter.route("mock-fast")).isEqualTo("mock-fast");
    }

    @Test
    void shouldReturnMockSmartWhenRequestedModelIsMockSmart() {
        assertThat(modelRouter.route("mock-smart")).isEqualTo("mock-smart");
    }

    @Test
    void shouldFallbackToDefaultModelWhenRequestedModelIsUnknown() {
        assertThat(modelRouter.route("unknown-model")).isEqualTo("mock-llm");
    }
}
