package com.tuoman.ai_task_orchestrator.vectorindex;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.VectorGenerationStatus;
import com.tuoman.ai_task_orchestrator.entity.VectorIndexGenerationEntity;
import org.springframework.stereotype.Component;

import java.util.Objects;

/**
 * V16 向量命名空间安全边界。
 *
 * 该 guard 在写入 VectorStore 前核对 request、Document、Chunk、VectorIdentity 和 payload。
 * 任一 collection、document、generation、dimension 不一致都直接拒绝写入，不能自动纠正。
 *
 * 关键不变量：namespace mismatch 一旦放过，就会让错误 collection 或错误版本进入检索结果，
 * 后续 filter、citation verification 都无法可靠补救。
 */
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
        // collectionId 必须贯穿 request -> identity -> chunk -> payload。
        // 缺失或不一致都意味着向量可能写入错误知识库，必须在入库前失败。
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

        // PURGED 是不可恢复删除，不能再写 active vector。
        // TRASHED 即使保留向量，也必须带 TRASHED status，保证默认检索过滤能排除。
        if (document.getLifecycleStatus() == DocumentLifecycleStatus.PURGED) {
            throw BusinessException.vectorNamespaceViolation("PURGED 文档不允许写入 active vector");
        }
        if (document.getLifecycleStatus() == DocumentLifecycleStatus.TRASHED
                && !"TRASHED".equals(payload.get("status"))) {
            throw BusinessException.vectorNamespaceViolation("TRASHED 文档写入 vector 时 status 必须为 TRASHED");
        }

        // dimension mismatch 通常来自模型切换或配置错误。
        // 自动截断/补零会破坏相似度语义，因此只能拒绝。
        if (vectorLength != identity.getEmbeddingDimension()) {
            throw BusinessException.vectorDimensionMismatch("向量维度与 embedding_dimension 不一致");
        }

        if (buildingOrActiveGeneration == null || !Objects.equals(buildingOrActiveGeneration, identity.getGeneration())) {
            throw BusinessException.vectorGenerationInvalid("generation 不是当前允许写入的代际");
        }
    }

    public void validateGenerationWritable(VectorIndexGenerationEntity generationEntity) {
        // 只有 BUILDING 或 ACTIVE generation 允许写入。
        // RETIRED/FAILED generation 重新写入会让重建和回滚语义失效。
        if (generationEntity == null) {
            throw BusinessException.vectorGenerationInvalid("generation 记录不存在");
        }
        if (generationEntity.getStatus() != VectorGenerationStatus.BUILDING
                && generationEntity.getStatus() != VectorGenerationStatus.ACTIVE) {
            throw BusinessException.vectorGenerationInvalid("generation 状态不允许写入: " + generationEntity.getStatus());
        }
    }
}
