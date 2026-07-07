package com.tuoman.ai_task_orchestrator.vectorindex;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.VectorGenerationStatus;
import com.tuoman.ai_task_orchestrator.entity.VectorIndexGenerationEntity;
import org.springframework.stereotype.Component;

import java.util.Objects;

@Component
public class VectorNamespaceGuard {

    public void validateUpsert(
            Long requestCollectionId,
            Long requestDocumentId,
            Long buildingOrActiveGeneration,
            DocumentEntity document,
            DocumentChunkEntity chunk,
            VectorIdentity identity,
            java.util.Map<String, String> payload,
            int vectorLength
    ) {
        if (identity.getCollectionId() == null) {
            throw BusinessException.vectorNamespaceViolation("collection_id 不能为空");
        }
        if (!Objects.equals(requestCollectionId, identity.getCollectionId())) {
            throw BusinessException.vectorCollectionMismatch("请求 collection_id 与 identity 不一致");
        }
        if (chunk.getCollectionId() != null && !Objects.equals(chunk.getCollectionId(), identity.getCollectionId())) {
            throw BusinessException.vectorCollectionMismatch("chunk.collection_id 与 identity 不一致");
        }
        if (!Objects.equals(requestDocumentId, identity.getDocumentId())) {
            throw BusinessException.vectorDocumentMismatch("请求 document_id 与 identity 不一致");
        }
        if (!Objects.equals(chunk.getDocumentId(), identity.getDocumentId())) {
            throw BusinessException.vectorDocumentMismatch("chunk.document_id 与 identity 不一致");
        }
        if (!Objects.equals(payload.get("collection_id"), String.valueOf(identity.getCollectionId()))) {
            throw BusinessException.vectorCollectionMismatch("payload.collection_id 与 identity 不一致");
        }
        if (!Objects.equals(payload.get("document_id"), String.valueOf(identity.getDocumentId()))) {
            throw BusinessException.vectorDocumentMismatch("payload.document_id 与 identity 不一致");
        }

        if (document.getLifecycleStatus() == DocumentLifecycleStatus.PURGED) {
            throw BusinessException.vectorNamespaceViolation("PURGED 文档不允许写入 active vector");
        }
        if (document.getLifecycleStatus() == DocumentLifecycleStatus.TRASHED
                && !"TRASHED".equals(payload.get("status"))) {
            throw BusinessException.vectorNamespaceViolation("TRASHED 文档写入 vector 时 status 必须为 TRASHED");
        }

        if (vectorLength != identity.getEmbeddingDimension()) {
            throw BusinessException.vectorDimensionMismatch("向量维度与 embedding_dimension 不一致");
        }

        if (buildingOrActiveGeneration == null || !Objects.equals(buildingOrActiveGeneration, identity.getGeneration())) {
            throw BusinessException.vectorGenerationInvalid("generation 不是当前允许写入的代际");
        }
    }

    public void validateGenerationWritable(VectorIndexGenerationEntity generationEntity) {
        if (generationEntity == null) {
            throw BusinessException.vectorGenerationInvalid("generation 记录不存在");
        }
        if (generationEntity.getStatus() != VectorGenerationStatus.BUILDING
                && generationEntity.getStatus() != VectorGenerationStatus.ACTIVE) {
            throw BusinessException.vectorGenerationInvalid("generation 状态不允许写入: " + generationEntity.getStatus());
        }
    }
}
