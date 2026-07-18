package com.tuoman.ai_task_orchestrator.staticresource;

import org.junit.jupiter.api.Test;

import java.io.IOException;
import java.nio.charset.StandardCharsets;

import static org.assertj.core.api.Assertions.assertThat;

class V19MemoryUiDocumentationTest {

    @Test
    void memoryCenterShouldContainRequiredChineseCopy() throws IOException {
        String html = resource("static/memory-center.html");
        assertThat(html)
                .contains("记忆中心")
                .contains("用户记忆")
                .contains("项目记忆")
                .contains("Agent 记忆")
                .contains("共享记忆")
                .contains("冲突记忆");
    }

    @Test
    void agentProfilesShouldContainDefaultRoleCopy() throws IOException {
        String html = resource("static/agent-profiles.html");
        String migration = resource("db/migration/V33__memory_foundation_and_agent_profiles.sql");
        assertThat(html).contains("智能体角色");
        assertThat(migration)
                .contains("产品经理 Agent")
                .contains("架构师 Agent")
                .contains("RAG 工程 Agent")
                .contains("风险审查 Agent");
    }

    private String resource(String path) throws IOException {
        try (var stream = getClass().getClassLoader().getResourceAsStream(path)) {
            assertThat(stream).as(path).isNotNull();
            return new String(stream.readAllBytes(), StandardCharsets.UTF_8);
        }
    }
}
