package com.tuoman.ai_task_orchestrator.vectorstore;

import java.util.List;

public interface VectorStore {

    void upsert(List<VectorStoreDocument> documents);

    List<VectorSearchResult> search(VectorSearchRequest request);

    void deleteByDocumentId(Long documentId);

    void deleteByDocumentIdAndProviderAndModel(Long documentId, String provider, String model);

    default VectorStoreOperationResult upsertOne(VectorStoreDocument document) {
        upsert(List.of(document));
        return VectorStoreOperationResult.success(1);
    }

    default VectorStoreOperationResult deleteByVectorId(String vectorId) {
        return VectorStoreOperationResult.unsupported("deleteByVectorId 当前向量库实现不支持");
    }

    default VectorStoreOperationResult deleteByStableVectorKey(String stableVectorKey) {
        return VectorStoreOperationResult.unsupported("deleteByStableVectorKey 当前向量库实现不支持");
    }

    default VectorStoreOperationResult deleteByDocumentIdScoped(Long collectionId, Long documentId) {
        if (collectionId == null || documentId == null) {
            throw new IllegalArgumentException("collectionId and documentId are required");
        }
        deleteByDocumentId(documentId);
        return VectorStoreOperationResult.success(1);
    }

    default VectorStoreOperationResult deleteByCollectionId(Long collectionId) {
        return VectorStoreOperationResult.unsupported("deleteByCollectionId 当前向量库实现不支持");
    }

    default VectorStoreOperationResult deleteByGeneration(Long collectionId, Long generation) {
        return VectorStoreOperationResult.unsupported("deleteByGeneration 当前向量库实现不支持");
    }

    default VectorStoreOperationResult deleteByStatus(String status) {
        return VectorStoreOperationResult.unsupported("deleteByStatus 当前向量库实现不支持");
    }

    default VectorStoreOperationResult deleteByFilter(VectorDeleteFilter filter) {
        return VectorStoreOperationResult.unsupported("deleteByFilter 当前向量库实现不支持");
    }

    default long countByFilter(VectorCountFilter filter) {
        return 0L;
    }

    default List<VectorStoreDocument> scanByFilter(VectorScanFilter filter) {
        return List.of();
    }
}
