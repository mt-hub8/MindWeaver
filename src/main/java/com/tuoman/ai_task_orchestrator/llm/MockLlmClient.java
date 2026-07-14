package com.tuoman.ai_task_orchestrator.llm;

import lombok.extern.slf4j.Slf4j;

@Slf4j
/**
 * V1.0 mock LLM client。
 *
 * 用确定性响应验证 Task 异步执行、Prompt Template、usage metadata 和 output chunks 链路。
 * mock 输出只代表链路可运行，不代表真实模型质量或事实正确性。
 */
public class MockLlmClient implements LlmClient {

    private static final String DEFAULT_MODEL = "mock-llm";

    private static final String PROVIDER = "mock";

    @Override
    public LlmResponse generate(LlmRequest request) {
        // 模拟 usage 和 latency，便于 V1.2 元数据持久化测试。
        // 这些数值是诊断占位，不应用于真实成本统计。
        long startTime = System.currentTimeMillis();

        if (request == null) {
            log.info("Generate mock LLM response, taskId=null, model=null");
            return failureResponse(null, DEFAULT_MODEL, "LLM request is null", "", startTime);
        }

        String model = normalizeModel(request.getModel());
        log.info("Generate mock LLM response, taskId={}, model={}", request.getTaskId(), model);

        String prompt = request.getPrompt();
        if (prompt == null || prompt.isBlank()) {
            return failureResponse(request.getTaskId(), model, "Prompt is empty", prompt, startTime);
        }

        if (prompt.contains("fail") || prompt.contains("失败")) {
            log.warn("Mock LLM execution failed, taskId={}, model={}", request.getTaskId(), model);
            return failureResponse(request.getTaskId(), model, "Mock LLM execution failed", prompt, startTime);
        }

        if (prompt.contains("用户问题：") && prompt.contains("上下文：")) {
            return ragCitationResponse(request, model, prompt, startTime);
        }

        String content = "Mock LLM response for prompt: " + prompt;
        int promptTokenCount = estimateTokenCount(prompt);
        int completionTokenCount = estimateTokenCount(content);

        LlmResponse response = new LlmResponse();
        response.setTaskId(request.getTaskId());
        response.setModel(model);
        response.setProvider(PROVIDER);
        response.setContent(content);
        response.setSuccess(true);
        response.setErrorMessage(null);
        response.setPromptTokenCount(promptTokenCount);
        response.setCompletionTokenCount(completionTokenCount);
        response.setTotalTokenCount(promptTokenCount + completionTokenCount);
        response.setLatencyMs(System.currentTimeMillis() - startTime);

        log.info("Mock LLM response generated, taskId={}, model={}", request.getTaskId(), model);
        return response;
    }

    private LlmResponse ragCitationResponse(LlmRequest request, String model, String prompt, long startTime) {
        int citationCount = countCitationMarkers(prompt);
        StringBuilder references = new StringBuilder();
        for (int index = 1; index <= citationCount; index++) {
            if (index > 1) {
                references.append(' ');
            }
            references.append('[').append(index).append(']');
        }

        String content = "根据检索到的上下文，问题与以下来源相关：" + references;
        int promptTokenCount = estimateTokenCount(prompt);
        int completionTokenCount = estimateTokenCount(content);

        LlmResponse response = new LlmResponse();
        response.setTaskId(request.getTaskId());
        response.setModel(model);
        response.setProvider(PROVIDER);
        response.setContent(content);
        response.setSuccess(true);
        response.setErrorMessage(null);
        response.setPromptTokenCount(promptTokenCount);
        response.setCompletionTokenCount(completionTokenCount);
        response.setTotalTokenCount(promptTokenCount + completionTokenCount);
        response.setLatencyMs(System.currentTimeMillis() - startTime);
        return response;
    }

    private int countCitationMarkers(String prompt) {
        int count = 0;
        int index = 1;
        while (prompt.contains("[" + index + "]")) {
            count++;
            index++;
        }
        return count;
    }

    private LlmResponse failureResponse(
            Long taskId,
            String model,
            String errorMessage,
            String prompt,
            long startTime
    ) {
        int promptTokenCount = estimateTokenCount(prompt);

        LlmResponse response = new LlmResponse();
        response.setTaskId(taskId);
        response.setModel(model);
        response.setProvider(PROVIDER);
        response.setContent(null);
        response.setSuccess(false);
        response.setErrorMessage(errorMessage);
        response.setPromptTokenCount(promptTokenCount);
        response.setCompletionTokenCount(0);
        response.setTotalTokenCount(promptTokenCount);
        response.setLatencyMs(System.currentTimeMillis() - startTime);
        return response;
    }

    private String normalizeModel(String model) {
        if (model == null || model.isBlank()) {
            return DEFAULT_MODEL;
        }
        return model;
    }

    private int estimateTokenCount(String text) {
        if (text == null || text.isBlank()) {
            return 1;
        }
        return Math.max(1, text.length() / 4);
    }
}
