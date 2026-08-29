package com.tuoman.ai_task_orchestrator.vectorstore.qdrant;

import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchFilter;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchRequest;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchResult;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreDocument;
import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;

@Component
public class QdrantPayloadMapper {

    private static final String METADATA_PREFIX = "metadata.";

    public QdrantPoint toPoint(VectorStoreDocument document) {
        validateDocument(document);

        Map<String, Object> payload = new LinkedHashMap<>();
        payload.put("chunkId", document.chunkId());
        payload.put("documentId", document.documentId());
        payload.put("content", document.content());
        payload.put("provider", document.provider());
        payload.put("model", document.model());
        payload.put("dimension", document.dimension());
        payload.put("distanceMetric", document.distanceMetric());
        payload.put("metadata", document.metadata() == null ? Map.of() : document.metadata());

        return new QdrantPoint(document.chunkId(), document.embedding(), payload);
    }

    public QdrantSearchRequest toSearchRequest(VectorSearchRequest request) {
        validateSearchRequest(request);
        return new QdrantSearchRequest(
                request.queryEmbedding(),
                request.topK(),
                true,
                toFilter(request)
        );
    }

    public QdrantDeletePointsRequest toDeleteByDocumentIdRequest(Long documentId) {
        if (documentId == null) {
            throw new QdrantVectorStoreException("documentId must not be null");
        }
        return new QdrantDeletePointsRequest(new QdrantFilter(List.of(
                new QdrantCondition("documentId", QdrantMatch.value(documentId))
        )));
    }

    public QdrantDeletePointsRequest toDeleteByDocumentIdAndProviderAndModelRequest(
            Long documentId,
            String provider,
            String model
    ) {
        if (documentId == null) {
            throw new QdrantVectorStoreException("documentId must not be null");
        }
        if (isBlank(provider)) {
            throw new QdrantVectorStoreException("provider must not be blank");
        }
        if (isBlank(model)) {
            throw new QdrantVectorStoreException("model must not be blank");
        }
        return new QdrantDeletePointsRequest(new QdrantFilter(List.of(
                new QdrantCondition("documentId", QdrantMatch.value(documentId)),
                new QdrantCondition("provider", QdrantMatch.value(provider)),
                new QdrantCondition("model", QdrantMatch.value(model))
        )));
    }

    public VectorSearchResult toSearchResult(QdrantScoredPoint point, int rank) {
        if (point == null || point.payload() == null) {
            throw new QdrantVectorStoreException("Qdrant scored point payload must not be null");
        }

        Map<String, Object> payload = point.payload();
        Long chunkId = requiredLong(payload, "chunkId");
        Long documentId = requiredLong(payload, "documentId");
        String content = requiredString(payload, "content");
        String provider = requiredString(payload, "provider");
        String model = requiredString(payload, "model");
        Integer dimension = requiredInteger(payload, "dimension");
        String distanceMetric = stringValue(payload.getOrDefault("distanceMetric", "COSINE"));
        Map<String, String> metadata = metadata(payload.get("metadata"));

        return new VectorSearchResult(
                chunkId,
                documentId,
                optionalInteger(metadata.get("chunkIndex")),
                content,
                optionalInteger(metadata.get("contentLength"), content.length()),
                metadata.get("headingPath"),
                optionalInteger(metadata.get("startOffset")),
                optionalInteger(metadata.get("endOffset")),
                metadata.get("chunkStrategy"),
                point.score(),
                rank,
                provider,
                model,
                dimension,
                distanceMetric,
                metadata
        );
    }

    private QdrantFilter toFilter(VectorSearchRequest request) {
        List<QdrantCondition> conditions = new ArrayList<>();
        conditions.add(new QdrantCondition("provider", QdrantMatch.value(request.provider())));
        conditions.add(new QdrantCondition("model", QdrantMatch.value(request.model())));
        conditions.add(new QdrantCondition("dimension", QdrantMatch.value(request.dimension())));

        VectorSearchFilter filter = request.filter() == null ? VectorSearchFilter.empty() : request.filter();
        if (filter.documentIds() != null && !filter.documentIds().isEmpty()) {
            conditions.add(new QdrantCondition(
                    "documentId",
                    QdrantMatch.any(filter.documentIds().stream().filter(Objects::nonNull).toList())
            ));
        }
        if (filter.metadataEquals() != null && !filter.metadataEquals().isEmpty()) {
            filter.metadataEquals().forEach((key, value) -> {
                if (key != null && !key.isBlank()) {
                    conditions.add(new QdrantCondition(METADATA_PREFIX + key, QdrantMatch.value(value)));
                }
            });
        }

        return new QdrantFilter(conditions);
    }

    private void validateDocument(VectorStoreDocument document) {
        if (document == null) {
            throw new QdrantVectorStoreException("vector store document must not be null");
        }
        if (document.chunkId() == null) {
            throw new QdrantVectorStoreException("chunkId must not be null");
        }
        if (document.documentId() == null) {
            throw new QdrantVectorStoreException("documentId must not be null");
        }
        if (document.embedding() == null || document.embedding().isEmpty()) {
            throw new QdrantVectorStoreException("embedding must not be empty");
        }
        if (document.dimension() == null || document.dimension() <= 0) {
            throw new QdrantVectorStoreException("dimension must be greater than 0");
        }
        if (document.embedding().size() != document.dimension()) {
            throw new QdrantVectorStoreException("embedding dimension must match document dimension");
        }
        if (isBlank(document.provider())) {
            throw new QdrantVectorStoreException("provider must not be blank");
        }
        if (isBlank(document.model())) {
            throw new QdrantVectorStoreException("model must not be blank");
        }
    }

    private void validateSearchRequest(VectorSearchRequest request) {
        if (request == null) {
            throw new QdrantVectorStoreException("vector search request must not be null");
        }
        if (request.queryEmbedding() == null || request.queryEmbedding().isEmpty()) {
            throw new QdrantVectorStoreException("queryEmbedding must not be empty");
        }
        if (request.topK() == null || request.topK() <= 0) {
            throw new QdrantVectorStoreException("topK must be greater than 0");
        }
        if (request.dimension() == null || request.dimension() <= 0) {
            throw new QdrantVectorStoreException("dimension must be greater than 0");
        }
        if (request.queryEmbedding().size() != request.dimension()) {
            throw new QdrantVectorStoreException("queryEmbedding dimension must match request dimension");
        }
        if (isBlank(request.provider())) {
            throw new QdrantVectorStoreException("provider must not be blank");
        }
        if (isBlank(request.model())) {
            throw new QdrantVectorStoreException("model must not be blank");
        }
    }

    private Long requiredLong(Map<String, Object> payload, String key) {
        Object value = payload.get(key);
        if (value instanceof Number number) {
            return number.longValue();
        }
        if (value instanceof String text && !text.isBlank()) {
            return Long.parseLong(text);
        }
        throw new QdrantVectorStoreException("Qdrant payload missing required field: " + key);
    }

    private Integer requiredInteger(Map<String, Object> payload, String key) {
        Object value = payload.get(key);
        if (value instanceof Number number) {
            return number.intValue();
        }
        if (value instanceof String text && !text.isBlank()) {
            return Integer.parseInt(text);
        }
        throw new QdrantVectorStoreException("Qdrant payload missing required field: " + key);
    }

    private String requiredString(Map<String, Object> payload, String key) {
        String value = stringValue(payload.get(key));
        if (isBlank(value)) {
            throw new QdrantVectorStoreException("Qdrant payload missing required field: " + key);
        }
        return value;
    }

    @SuppressWarnings("unchecked")
    private Map<String, String> metadata(Object rawMetadata) {
        if (!(rawMetadata instanceof Map<?, ?> rawMap)) {
            return Map.of();
        }

        Map<String, String> metadata = new LinkedHashMap<>();
        rawMap.forEach((key, value) -> {
            if (key != null && value != null) {
                metadata.put(String.valueOf(key), String.valueOf(value));
            }
        });
        return metadata;
    }

    private Integer optionalInteger(String value) {
        return optionalInteger(value, null);
    }

    private Integer optionalInteger(String value, Integer defaultValue) {
        if (value == null || value.isBlank()) {
            return defaultValue;
        }
        return Integer.parseInt(value);
    }

    private String stringValue(Object value) {
        return value == null ? null : String.valueOf(value);
    }

    private boolean isBlank(String value) {
        return value == null || value.isBlank();
    }
}
