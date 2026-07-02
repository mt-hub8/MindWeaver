package com.tuoman.ai_task_orchestrator.agent.tool;

import com.tuoman.ai_task_orchestrator.llm.LlmGenerateOptions;
import com.tuoman.ai_task_orchestrator.llm.LlmGenerateResult;
import com.tuoman.ai_task_orchestrator.llm.LlmProvider;
import com.tuoman.ai_task_orchestrator.llm.MockLlmProvider;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.List;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class ContextSummaryToolTest {

    @Mock
    private LlmProvider llmProvider;

    @Test
    void executeShouldReturnNoSummaryForNoContext() {
        ContextSummaryTool tool = new ContextSummaryTool(llmProvider);
        ToolExecutionResult result = tool.execute(
                Map.of("noContext", true, "matchedChunks", List.of(), "taskObjective", "目标"),
                new ToolExecutionContext(1L, "t", "目标", null, null, "全部文档")
        );

        assertThat(result.success()).isTrue();
        assertThat(result.output().get("summary")).isEqualTo("无可总结内容");
        verify(llmProvider, never()).generate(anyString(), anyString(), any(LlmGenerateOptions.class));
    }

    @Test
    void executeShouldCallLlmForMatchedChunks() {
        when(llmProvider.generate(anyString(), anyString(), any(LlmGenerateOptions.class)))
                .thenReturn(LlmGenerateResult.success(
                        "- 要点一\n- 要点二",
                        MockLlmProvider.PROVIDER,
                        MockLlmProvider.DEFAULT_MODEL,
                        10,
                        5,
                        1L,
                        "stop",
                        null
                ));

        ContextSummaryTool tool = new ContextSummaryTool(llmProvider);
        ToolExecutionResult result = tool.execute(
                Map.of(
                        "matchedChunks",
                        List.of(Map.of("sourceIndex", 1, "snippet", "内容")),
                        "taskObjective",
                        "目标"
                ),
                new ToolExecutionContext(1L, "t", "目标", null, null, "全部文档")
        );

        assertThat(result.success()).isTrue();
        assertThat(result.output().get("summary")).isEqualTo("- 要点一\n- 要点二");
        verify(llmProvider).generate(anyString(), anyString(), any(LlmGenerateOptions.class));
    }
}
