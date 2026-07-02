package com.tuoman.ai_task_orchestrator.llm;

public interface LlmProvider {

    LlmGenerateResult generate(String systemPrompt, String userPrompt, LlmGenerateOptions options);

    String provider();

    String defaultModel();
}
