package com.tuoman.ai_task_orchestrator.vectorstore.qdrant;

import com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreProperties;

public interface QdrantVectorStoreClient {

    void createCollectionIfNeeded(
            VectorStoreProperties.Qdrant properties,
            QdrantCreateCollectionRequest request
    );

    void upsertPoints(
            VectorStoreProperties.Qdrant properties,
            QdrantUpsertPointsRequest request
    );

    QdrantSearchResponse searchPoints(
            VectorStoreProperties.Qdrant properties,
            QdrantSearchRequest request
    );

    void deletePoints(
            VectorStoreProperties.Qdrant properties,
            QdrantDeletePointsRequest request
    );
}
