package com.tuoman.ai_task_orchestrator.agent.tool;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.common.error.ErrorCode;
import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class AgentToolRegistryTest {

    @Test
    void shouldListEnabledToolsWithChineseDisplayNames() {
        AgentToolRegistry registry = new AgentToolRegistry(List.of(
                new StubTool(AgentToolNames.KNOWLEDGE_SEARCH, "检索知识库"),
                new StubTool(AgentToolNames.CONTEXT_SUMMARY, "总结检索结果")
        ));

        List<AgentTool> tools = registry.listEnabledTools();
        assertThat(tools).hasSize(2);
        assertThat(tools.get(0).displayName()).isEqualTo("检索知识库");
        assertThat(tools.get(1).displayName()).isEqualTo("总结检索结果");
    }

    @Test
    void findToolOrThrowShouldFailForUnknownTool() {
        AgentToolRegistry registry = new AgentToolRegistry(List.of());

        assertThatThrownBy(() -> registry.findToolOrThrow("unknown"))
                .isInstanceOf(BusinessException.class)
                .extracting("errorCode")
                .isEqualTo(ErrorCode.AGENT_TOOL_NOT_FOUND);
    }

    private static final class StubTool implements AgentTool {

        private final String name;
        private final String displayName;

        private StubTool(String name, String displayName) {
            this.name = name;
            this.displayName = displayName;
        }

        @Override
        public String toolName() {
            return name;
        }

        @Override
        public String displayName() {
            return displayName;
        }

        @Override
        public String description() {
            return displayName;
        }

        @Override
        public java.util.Map<String, Object> inputSchema() {
            return java.util.Map.of();
        }

        @Override
        public java.util.Map<String, Object> outputSchema() {
            return java.util.Map.of();
        }

        @Override
        public boolean enabled() {
            return true;
        }

        @Override
        public ToolExecutionResult execute(java.util.Map<String, Object> input, ToolExecutionContext context) {
            return ToolExecutionResult.success(java.util.Map.of(), 0L);
        }
    }
}
