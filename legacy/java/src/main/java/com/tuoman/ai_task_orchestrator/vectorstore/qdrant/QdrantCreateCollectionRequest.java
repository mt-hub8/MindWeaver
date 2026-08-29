package com.tuoman.ai_task_orchestrator.vectorstore.qdrant;

public record QdrantCreateCollectionRequest(
        VectorConfig vectors
) {

    public static QdrantCreateCollectionRequest cosine(int dimension) {
        return new QdrantCreateCollectionRequest(new VectorConfig(dimension, "Cosine"));
    }

    public record VectorConfig(
            int size,
            String distance
    ) {
    }
}
