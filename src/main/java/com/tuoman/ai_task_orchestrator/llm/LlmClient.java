package com.tuoman.ai_task_orchestrator.llm;

public interface LlmClient {

    LlmResponse generate(LlmRequest request);
}
