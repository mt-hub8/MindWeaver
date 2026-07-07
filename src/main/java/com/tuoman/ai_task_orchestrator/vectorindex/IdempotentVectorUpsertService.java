package com.tuoman.ai_task_orchestrator.vectorindex;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.VectorUpsertOperation;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkEmbeddingRepository;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStore;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreDocument;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.Optional;

@Service
@RequiredArgsConstructor
public class IdempotentVectorUpsertService {

    private final VectorIdentityService vectorIdentityService;

    private final VectorPayloadBuilder vectorPayloadBuilder;

    private final VectorNamespaceGuard vectorNamespaceGuard;

    private final VectorGenerationService vectorGenerationService;

    private final VectorStore vectorStore;

    private final DocumentChunkEmbeddingRepository documentChunkEmbeddingRepository;

    private final ObjectMapper objectMapper;

    public VectorUpsertResult upsert(VectorUpsertRequest request) {
        List<String> warnings = new ArrayList<>();
        DocumentEntity document = request.getDocument();
        DocumentChunkEntity chunk = request.getChunk();

        String chunkUid = chunk.getChunkUid();
        if (chunkUid == null || chunkUid.isBlank()) {
            chunkUid = document.getId() + "#" + chunk.getChunkIndex();
            warnings.add("chunk_uid 缺失，已使用 documentId#chunkIndex 降级");
        }

        Long collectionId = request.getCollectionId() != null ? request.getCollectionId() : chunk.getCollectionId();
        if (collectionId == null) {
            collectionId = 0L;
            warnings.add("collection_id 缺失，使用占位符 0 并记录 audit warning");
        }

        var generationEntity = vectorGenerationService.resolveWritableGeneration(
                collectionId,
                request.getDocumentId(),
                request.getGeneration()
        );
        vectorNamespaceGuard.validateGenerationWritable(generationEntity);
        Long effectiveGeneration = generationEntity.getGeneration();

        Map<String, String> preliminaryPayload = Map.of(
                "collection_id", String.valueOf(collectionId),
                "document_id", String.valueOf(request.getDocumentId()),
                "chunk_uid", chunkUid,
                "embedding_model", request.getEmbeddingModel(),
                "embedding_dimension", String.valueOf(request.getEmbeddingDimension()),
                "vector_generation", String.valueOf(effectiveGeneration)
        );
        String metadataCanonical = vectorPayloadBuilder.canonicalMetadata(preliminaryPayload);

        VectorIdentity identity = vectorIdentityService.build(
                collectionId,
                request.getDocumentId(),
                chunk.getId(),
                chunkUid,
                request.getEmbeddingModel(),
                request.getEmbeddingDimension(),
                effectiveGeneration,
                chunk.getContent(),
                metadataCanonical
        );

        Map<String, String> payload = vectorPayloadBuilder.build(
                identity,
                document,
                chunk,
                request.getDocumentGeneration(),
                request.getChunkGeneration(),
                effectiveGeneration
        );

        vectorPayloadBuilder.validateVectorLength(request.getEmbeddingVector().size(), request.getEmbeddingDimension());
        vectorNamespaceGuard.validateUpsert(
                collectionId,
                request.getDocumentId(),
                effectiveGeneration,
                document,
                chunk,
                identity,
                payload,
                request.getEmbeddingVector().size()
        );

        Optional<com.tuoman.ai_task_orchestrator.entity.DocumentChunkEmbeddingEntity> existing =
                documentChunkEmbeddingRepository.findByVectorId(identity.getVectorId());

        VectorStoreDocument documentToStore = new VectorStoreDocument(
                chunk.getId(),
                request.getDocumentId(),
                chunk.getContent(),
                request.getEmbeddingVector(),
                request.getEmbeddingProvider(),
                request.getEmbeddingModel(),
                request.getEmbeddingDimension(),
                request.getDistanceMetric(),
                payload,
                identity.getVectorId(),
                identity.getStableVectorKey(),
                collectionId,
                chunkUid,
                effectiveGeneration
        );

        vectorStore.upsert(List.of(documentToStore));

        VectorUpsertOperation operation = existing.isPresent() ? VectorUpsertOperation.UPDATED : VectorUpsertOperation.CREATED;
        return VectorUpsertResult.builder()
                .vectorId(identity.getVectorId())
                .stableVectorKey(identity.getStableVectorKey())
                .operation(operation)
                .collectionId(collectionId)
                .documentId(request.getDocumentId())
                .chunkId(chunk.getId())
                .generation(effectiveGeneration)
                .embeddingModel(request.getEmbeddingModel())
                .embeddingDimension(request.getEmbeddingDimension())
                .status(payload.get("status"))
                .warningMessages(warnings)
                .build();
    }

    public List<VectorUpsertResult> upsertBatch(List<VectorUpsertRequest> requests) {
        List<VectorUpsertResult> results = new ArrayList<>();
        for (VectorUpsertRequest request : requests) {
            results.add(upsert(request));
        }
        return results;
    }

    String serializePayload(Map<String, String> payload) {
        try {
            return objectMapper.writeValueAsString(payload);
        } catch (JsonProcessingException exception) {
            throw BusinessException.vectorPayloadInvalid("payload 序列化失败");
        }
    }
}
