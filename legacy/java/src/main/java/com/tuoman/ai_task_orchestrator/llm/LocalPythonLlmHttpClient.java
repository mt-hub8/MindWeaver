package com.tuoman.ai_task_orchestrator.llm;

public interface LocalPythonLlmHttpClient {

    LocalPythonLlmResponse generate(LocalPythonLlmRequest request, LlmProperties properties);
}
