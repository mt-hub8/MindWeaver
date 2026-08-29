package com.tuoman.ai_task_orchestrator.vectorstore;

import java.util.List;
import java.util.Map;

public record VectorStoreDocument(
        Long chunkId,
        Long documentId,
        String content,
        List<Double> embedding,
        String provider,
        String model,
        Integer dimension,
        String distanceMetric,
        Map<String, String> metadata,
        String vectorId,
        String stableVectorKey,
        Long collectionId,
        String chunkUid,
        Long vectorGeneration
) {
    public VectorStoreDocument {
        if (metadata == null) {
            metadata = Map.of();
        }
    }

    public static VectorStoreDocument legacy(
            Long chunkId,
            Long documentId,
            String content,
            List<Double> embedding,
            String provider,
            String model,
            Integer dimension,
            String distanceMetric,
            Map<String, String> metadata
    ) {
        return new VectorStoreDocument(
                chunkId,
                documentId,
                content,
                embedding,
                provider,
                model,
                dimension,
                distanceMetric,
                metadata,
                null,
                null,
                metadata != null && metadata.get("collectionId") != null
                        ? Long.valueOf(metadata.get("collectionId"))
                        : null,
                null,
                metadata != null && metadata.get("generation") != null
                        ? Long.valueOf(metadata.get("generation"))
                        : null
        );
    }

    public static VectorStoreDocument of(
            Long chunkId,
            Long documentId,
            String content,
            List<Double> embedding,
            String provider,
            String model,
            Integer dimension,
            String distanceMetric,
            Map<String, String> metadata
    ) {
        return legacy(chunkId, documentId, content, embedding, provider, model, dimension, distanceMetric, metadata);
    }
}
