package com.tuoman.ai_task_orchestrator.embedding;

public interface EmbeddingClient {

    EmbeddingResponse embed(EmbeddingRequest request);
}
