package com.tuoman.ai_task_orchestrator.vectorstore.qdrant;

import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchRequest;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchResult;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStore;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreDocument;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreProperties;

import java.util.List;
import java.util.Objects;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.stream.IntStream;

/**
 * Qdrant 向量库适配器。
 *
 * 该类只负责把统一的 VectorStoreDocument / VectorSearchRequest 映射到 Qdrant API；
 * collection、generation、status 等隔离语义来自上游 payload 和 mapper，不应在这里被绕过。
 */
public class QdrantVectorStore implements VectorStore {

    public static final String PROVIDER = "qdrant";

    private final VectorStoreProperties.Qdrant properties;

    private final QdrantVectorStoreClient client;

    private final QdrantPayloadMapper mapper;

    private final AtomicBoolean collectionInitialized = new AtomicBoolean(false);

    public QdrantVectorStore(
            VectorStoreProperties.Qdrant properties,
            QdrantVectorStoreClient client,
            QdrantPayloadMapper mapper
    ) {
        this.properties = properties;
        this.client = client;
        this.mapper = mapper;
        validateProperties(properties);
    }

    @Override
    public void upsert(List<VectorStoreDocument> documents) {
        if (documents == null || documents.isEmpty()) {
            return;
        }

        List<VectorStoreDocument> safeDocuments = documents.stream()
                .filter(Objects::nonNull)
                .toList();
        if (safeDocuments.isEmpty()) {
            return;
        }

        // Qdrant collection schema 绑定 dimension，同批写入必须同维度。
        // 模型切换导致维度变化时，应通过 reindex 建新索引，而不是混写。
        int dimension = validateSameDimension(safeDocuments);
        initializeCollectionIfNeeded(dimension);

        try {
            client.upsertPoints(
                    properties,
                    new QdrantUpsertPointsRequest(safeDocuments.stream()
                            .map(mapper::toPoint)
                            .toList())
            );
        } catch (QdrantVectorStoreException exception) {
            throw exception;
        } catch (Exception exception) {
            throw new QdrantVectorStoreException("Qdrant vector store upsert failed", exception);
        }
    }

    @Override
    public List<VectorSearchResult> search(VectorSearchRequest request) {
        QdrantSearchRequest searchRequest = mapper.toSearchRequest(request);
        initializeCollectionIfNeeded(request.dimension());

        QdrantSearchResponse response;
        try {
            response = client.searchPoints(properties, searchRequest);
        } catch (QdrantVectorStoreException exception) {
            throw exception;
        } catch (Exception exception) {
            throw new QdrantVectorStoreException("Qdrant vector store search failed", exception);
        }

        if (response == null || response.result() == null || response.result().isEmpty()) {
            return List.of();
        }

        return IntStream.range(0, response.result().size())
                .mapToObj(index -> mapper.toSearchResult(response.result().get(index), index + 1))
                .toList();
    }

    @Override
    public void deleteByDocumentId(Long documentId) {
        try {
            client.deletePoints(properties, mapper.toDeleteByDocumentIdRequest(documentId));
        } catch (QdrantVectorStoreException exception) {
            throw exception;
        } catch (Exception exception) {
            throw new QdrantVectorStoreException("Qdrant vector store delete failed", exception);
        }
    }

    @Override
    public void deleteByDocumentIdAndProviderAndModel(Long documentId, String provider, String model) {
        try {
            client.deletePoints(properties, mapper.toDeleteByDocumentIdAndProviderAndModelRequest(
                    documentId,
                    provider,
                    model
            ));
        } catch (QdrantVectorStoreException exception) {
            throw exception;
        } catch (Exception exception) {
            throw new QdrantVectorStoreException("Qdrant vector store delete failed", exception);
        }
    }

    private void initializeCollectionIfNeeded(Integer dimension) {
        if (!properties.isInitializeCollection()) {
            return;
        }
        if (dimension == null || dimension <= 0) {
            throw new QdrantVectorStoreException("dimension must be greater than 0 to initialize Qdrant collection");
        }
        if (collectionInitialized.compareAndSet(false, true)) {
            client.createCollectionIfNeeded(properties, QdrantCreateCollectionRequest.cosine(dimension));
        }
    }

    private int validateSameDimension(List<VectorStoreDocument> documents) {
        Integer dimension = documents.getFirst().dimension();
        if (dimension == null || dimension <= 0) {
            throw new QdrantVectorStoreException("dimension must be greater than 0");
        }
        boolean mismatch = documents.stream()
                .anyMatch(document -> !dimension.equals(document.dimension()));
        if (mismatch) {
            throw new QdrantVectorStoreException("Qdrant upsert documents must have the same dimension");
        }
        return dimension;
    }

    private void validateProperties(VectorStoreProperties.Qdrant properties) {
        if (properties == null) {
            throw new QdrantVectorStoreException("Qdrant properties must not be null");
        }
        if (properties.getBaseUrl() == null || properties.getBaseUrl().isBlank()) {
            throw new QdrantVectorStoreException("Qdrant base-url must not be blank");
        }
        if (properties.getCollectionName() == null || properties.getCollectionName().isBlank()) {
            throw new QdrantVectorStoreException("Qdrant collection-name must not be blank");
        }
        if (properties.getTimeoutMs() <= 0) {
            throw new QdrantVectorStoreException("Qdrant timeout-ms must be greater than 0");
        }
    }
}
