package com.tuoman.ai_task_orchestrator.llm;

import com.tuoman.ai_task_orchestrator.modelprovider.ResolvedModelProvider;

public interface OpenAiCompatibleLlmHttpClient {

    OpenAiChatResponse createChatCompletion(OpenAiChatRequest request, ResolvedModelProvider provider);
}
