package com.tuoman.ai_task_orchestrator.llm;

import lombok.RequiredArgsConstructor;

@RequiredArgsConstructor
public class DelegatingLlmClient implements LlmClient {

    private final LlmProvider llmProvider;

    private final LlmProperties llmProperties;

    @Override
    public LlmResponse generate(LlmRequest request) {
        LlmGenerateOptions options = new LlmGenerateOptions();
        if (request != null) {
            options.setTaskId(request.getTaskId());
            options.setModel(request.getModel());
            options.setTemperature(llmProperties.getTemperature());
            options.setMaxTokens(llmProperties.getMaxTokens());
        }

        String prompt = request == null ? "" : request.getPrompt();
        LlmGenerateResult result = llmProvider.generate(null, prompt, options);

        LlmResponse response = new LlmResponse();
        if (request != null) {
            response.setTaskId(request.getTaskId());
        }
        response.setModel(result.getModel());
        response.setProvider(result.getProvider());
        response.setContent(result.getContent());
        response.setSuccess(result.isSuccess());
        response.setErrorMessage(result.getErrorMessage());
        response.setPromptTokenCount(result.getInputTokens());
        response.setCompletionTokenCount(result.getOutputTokens());
        if (result.getInputTokens() != null && result.getOutputTokens() != null) {
            response.setTotalTokenCount(result.getInputTokens() + result.getOutputTokens());
        }
        response.setLatencyMs(result.getLatencyMs());
        return response;
    }
}
