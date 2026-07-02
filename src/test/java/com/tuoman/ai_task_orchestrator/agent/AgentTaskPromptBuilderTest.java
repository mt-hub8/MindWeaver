package com.tuoman.ai_task_orchestrator.agent;

import com.tuoman.ai_task_orchestrator.dto.AgentTaskCitationResponse;
import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

class AgentTaskPromptBuilderTest {

    private final AgentTaskPromptBuilder builder = new AgentTaskPromptBuilder();

    @Test
    void buildUserPromptShouldContainObjectiveAndContext() {
        String prompt = builder.buildUserPrompt(
                "总结系统核心功能",
                "全部文档",
                List.of(new AgentTaskCitationResponse(1, 10L, "demo.md", 100L, 0.9, "chunk content", null))
        );

        assertThat(prompt).contains("任务目标");
        assertThat(prompt).contains("总结系统核心功能");
        assertThat(prompt).contains("知识库上下文");
        assertThat(prompt).contains("chunk content");
        assertThat(prompt).contains("任务结论");
    }
}
