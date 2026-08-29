package com.tuoman.ai_task_orchestrator.embedding;

public interface LocalEmbeddingWorkerClient {

    LocalEmbeddingWorkerResponse createEmbeddings(
            LocalEmbeddingWorkerRequest request,
            EmbeddingProperties.LocalWorker properties
    );
}
