package com.tuoman.ai_task_orchestrator.vectorindex;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.ChunkMetadataStatus;
import com.tuoman.ai_task_orchestrator.enums.ChunkType;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import org.springframework.stereotype.Component;

import java.time.LocalDateTime;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.stream.Collectors;

@Component
public class VectorPayloadBuilder {

    public Map<String, String> build(
            VectorIdentity identity,
            DocumentEntity document,
            DocumentChunkEntity chunk,
            Long documentGeneration,
            Long chunkGeneration,
            Long vectorGeneration
    ) {
        if (identity == null) {
            throw BusinessException.vectorPayloadInvalid("vector identity 不能为空");
        }
        if (identity.getCollectionId() == null) {
            throw BusinessException.vectorPayloadInvalid("collection_id 不能为空");
        }
        if (identity.getDocumentId() == null) {
            throw BusinessException.vectorPayloadInvalid("document_id 不能为空");
        }
        if (identity.getChunkUid() == null || identity.getChunkUid().isBlank()) {
            throw BusinessException.vectorPayloadInvalid("chunk_uid 不能为空");
        }
        if (identity.getEmbeddingModel() == null || identity.getEmbeddingModel().isBlank()) {
            throw BusinessException.vectorPayloadInvalid("embedding_model 不能为空");
        }
        if (identity.getEmbeddingDimension() == null || identity.getEmbeddingDimension() <= 0) {
            throw BusinessException.vectorPayloadInvalid("embedding_dimension 必须大于 0");
        }
        if (vectorGeneration == null || vectorGeneration <= 0) {
            throw BusinessException.vectorPayloadInvalid("vector_generation 必须大于 0");
        }

        String status = resolveStatus(document, chunk);
        Map<String, String> payload = new LinkedHashMap<>();
        payload.put("vector_id", identity.getVectorId());
        payload.put("stable_vector_key", identity.getStableVectorKey());
        payload.put("collection_id", String.valueOf(identity.getCollectionId()));
        payload.put("document_id", String.valueOf(identity.getDocumentId()));
        payload.put("chunk_id", identity.getChunkId() == null ? "" : String.valueOf(identity.getChunkId()));
        payload.put("chunk_uid", identity.getChunkUid());
        payload.put("document_generation", documentGeneration == null ? "" : String.valueOf(documentGeneration));
        payload.put("chunk_generation", chunkGeneration == null ? "" : String.valueOf(chunkGeneration));
        payload.put("vector_generation", String.valueOf(vectorGeneration));
        payload.put("embedding_model", identity.getEmbeddingModel());
        payload.put("embedding_dimension", String.valueOf(identity.getEmbeddingDimension()));
        payload.put("content_hash", identity.getContentHash() == null ? "" : identity.getContentHash());
        payload.put("metadata_hash", identity.getMetadataHash() == null ? "" : identity.getMetadataHash());
        payload.put("doc_type", chunk.getDocType() == null ? "" : chunk.getDocType().name());
        payload.put("version", chunk.getVersion() == null ? "" : chunk.getVersion());
        payload.put("source", chunk.getSource() == null ? "" : chunk.getSource());
        payload.put("status", status);
        payload.put("section_path", chunk.getSectionPath() == null ? "" : chunk.getSectionPath());
        payload.put("chunk_type", chunk.getChunkType() == null ? ChunkType.UNKNOWN.name() : chunk.getChunkType().name());
        payload.put("created_at", LocalDateTime.now().toString());
        payload.put("updated_at", LocalDateTime.now().toString());
        return payload;
    }

    public String canonicalMetadata(Map<String, String> payload) {
        return payload.entrySet().stream()
                .sorted(Map.Entry.comparingByKey())
                .map(entry -> entry.getKey() + "=" + entry.getValue())
                .collect(Collectors.joining("&"));
    }

    public void validateVectorLength(int actualLength, Integer expectedDimension) {
        if (expectedDimension == null || actualLength != expectedDimension) {
            throw BusinessException.vectorDimensionMismatch(
                    "向量长度 " + actualLength + " 与 embedding_dimension " + expectedDimension + " 不一致"
            );
        }
    }

    private String resolveStatus(DocumentEntity document, DocumentChunkEntity chunk) {
        if (document.getLifecycleStatus() == DocumentLifecycleStatus.TRASHED) {
            return ChunkMetadataStatus.TRASHED.name();
        }
        if (document.getLifecycleStatus() == DocumentLifecycleStatus.PURGED) {
            return ChunkMetadataStatus.PURGED.name();
        }
        if (chunk.getMetadataStatus() != null) {
            return chunk.getMetadataStatus().name();
        }
        return ChunkMetadataStatus.ACTIVE.name();
    }
}
