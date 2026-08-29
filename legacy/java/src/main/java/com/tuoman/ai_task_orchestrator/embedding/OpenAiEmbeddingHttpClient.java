package com.tuoman.ai_task_orchestrator.embedding;

public interface OpenAiEmbeddingHttpClient {

    OpenAiEmbeddingResponse createEmbeddings(
            OpenAiEmbeddingRequest request,
            EmbeddingProperties.OpenAi properties
    );
}
