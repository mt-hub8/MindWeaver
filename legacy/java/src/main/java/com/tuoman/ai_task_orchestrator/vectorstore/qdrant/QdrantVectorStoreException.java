package com.tuoman.ai_task_orchestrator.vectorstore.qdrant;

public class QdrantVectorStoreException extends RuntimeException {

    public QdrantVectorStoreException(String message) {
        super(message);
    }

    public QdrantVectorStoreException(String message, Throwable cause) {
        super(message, cause);
    }
}
