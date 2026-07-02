package com.tuoman.ai_task_orchestrator.llm;

import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;

import java.util.Map;

@Slf4j
@Component
public class MockLlmProvider implements LlmProvider {

    public static final String PROVIDER = "mock";

    public static final String DEFAULT_MODEL = "mock-llm";

    @Override
    public LlmGenerateResult generate(String systemPrompt, String userPrompt, LlmGenerateOptions options) {
        long startTime = System.currentTimeMillis();
        String model = resolveModel(options);
        String user = userPrompt == null ? "" : userPrompt.trim();
        String system = systemPrompt == null ? "" : systemPrompt.trim();

        if (user.isBlank()) {
            return LlmGenerateResult.failure(PROVIDER, model, "Prompt is empty", elapsed(startTime));
        }

        if (user.contains("fail") || user.contains("失败")) {
            log.warn("Mock LLM generation failed by keyword");
            return LlmGenerateResult.failure(PROVIDER, model, "Mock LLM execution failed", elapsed(startTime));
        }

        String combined = system + "\n" + user;
        String content;
        if (user.contains("用户问题：") && user.contains("上下文：")) {
            content = ragStyleAnswer(user);
        } else if (user.contains("任务目标") || user.contains("知识库上下文")) {
            content = agentTaskStyleAnswer(user);
        } else {
            content = "Mock LLM response for prompt: " + user;
        }

        int inputTokens = estimateTokenCount(combined);
        int outputTokens = estimateTokenCount(content);
        return LlmGenerateResult.success(
                content,
                PROVIDER,
                model,
                inputTokens,
                outputTokens,
                elapsed(startTime),
                "stop",
                Map.of("mock", true)
        );
    }

    private String agentTaskStyleAnswer(String userPrompt) {
        return """
                ## 任务结论
                基于知识库上下文，已完成任务分析（Mock LLM）。

                ## 关键依据
                - 已参考检索到的文档片段。

                ## 风险 / 不确定性
                - 若上下文不足，请补充文档后重试。

                ## 下一步建议
                - 核对引用来源并验证关键事实。

                ## 引用来源
                - 见任务详情中的引用列表。
                """;
    }

    private String ragStyleAnswer(String prompt) {
        int citationCount = 0;
        int index = 1;
        while (prompt.contains("[" + index + "]")) {
            citationCount++;
            index++;
        }
        StringBuilder references = new StringBuilder();
        for (int i = 1; i <= citationCount; i++) {
            if (i > 1) {
                references.append(' ');
            }
            references.append('[').append(i).append(']');
        }
        return "根据检索到的上下文，问题与以下来源相关：" + references;
    }

    @Override
    public String provider() {
        return PROVIDER;
    }

    @Override
    public String defaultModel() {
        return DEFAULT_MODEL;
    }

    private String resolveModel(LlmGenerateOptions options) {
        if (options != null && options.getModel() != null && !options.getModel().isBlank()) {
            return options.getModel();
        }
        return DEFAULT_MODEL;
    }

    private long elapsed(long startTime) {
        return System.currentTimeMillis() - startTime;
    }

    private int estimateTokenCount(String text) {
        if (text == null || text.isBlank()) {
            return 1;
        }
        return Math.max(1, text.length() / 4);
    }
}
