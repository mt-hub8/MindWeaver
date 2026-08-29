package com.tuoman.ai_task_orchestrator.vectorindex;

import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.VectorGenerationStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkEmbeddingRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentCollectionRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.repository.VectorIndexGenerationRepository;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorDeleteFilter;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorScanFilter;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStore;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreDocument;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreOperationResult;
import lombok.Builder;
import lombok.Getter;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;

import java.util.ArrayList;
import java.util.List;
import java.util.Objects;
import java.util.Set;
import java.util.stream.Collectors;

/**
 * 向量清理服务。
 *
 * cleanup 是破坏性操作，和 VectorConsistencyAuditService 的只读诊断刻意分离。
 * 所有清理入口都必须带 collection、document 或 generation scope，禁止无范围全库删除。
 */
@Service
@RequiredArgsConstructor
public class VectorCleanupService {

    private final VectorStore vectorStore;

    private final DocumentRepository documentRepository;

    private final DocumentChunkRepository documentChunkRepository;

    private final DocumentChunkEmbeddingRepository documentChunkEmbeddingRepository;

    private final DocumentCollectionRepository documentCollectionRepository;

    private final VectorIndexGenerationRepository vectorIndexGenerationRepository;

    public VectorCleanupResult cleanupDocumentVectors(Long collectionId, Long documentId) {
        // document scope 删除用于 PURGED 或显式清理。
        // collectionId 必填，避免同 documentId 在不同 namespace 下被误删。
        if (collectionId == null || documentId == null) {
            throw new IllegalArgumentException("collectionId and documentId are required");
        }
        VectorStoreOperationResult result = vectorStore.deleteByDocumentIdScoped(collectionId, documentId);
        return VectorCleanupResult.builder()
                .requestedCount(1)
                .deletedCount(result.affectedCount())
                .warnings(new ArrayList<>(result.warnings()))
                .build();
    }

    public VectorCleanupResult cleanupCollectionVectors(Long collectionId) {
        if (collectionId == null) {
            throw new IllegalArgumentException("collectionId is required");
        }
        VectorStoreOperationResult result = vectorStore.deleteByCollectionId(collectionId);
        return VectorCleanupResult.builder()
                .requestedCount(1)
                .deletedCount(result.affectedCount())
                .warnings(new ArrayList<>(result.warnings()))
                .build();
    }

    public VectorCleanupResult cleanupGenerationVectors(Long collectionId, Long generation) {
        // generation scope 通常用于新 generation 激活后清理 RETIRED 向量。
        // BUILDING/ACTIVE 不应通过这个入口随意删除。
        if (collectionId == null || generation == null) {
            throw new IllegalArgumentException("collectionId and generation are required");
        }
        VectorStoreOperationResult result = vectorStore.deleteByGeneration(collectionId, generation);
        return VectorCleanupResult.builder()
                .requestedCount(1)
                .deletedCount(result.affectedCount())
                .warnings(new ArrayList<>(result.warnings()))
                .build();
    }

    public VectorCleanupResult cleanupRetiredGenerations(Long collectionId) {
        if (collectionId == null) {
            throw new IllegalArgumentException("collectionId is required");
        }
        List<String> warnings = new ArrayList<>();
        int deleted = 0;
        var retired = vectorIndexGenerationRepository.findByCollectionIdAndDocumentIdIsNullAndStatus(
                collectionId,
                VectorGenerationStatus.RETIRED
        );
        for (var generation : retired) {
            VectorStoreOperationResult result = vectorStore.deleteByGeneration(collectionId, generation.getGeneration());
            deleted += result.affectedCount();
            warnings.addAll(result.warnings());
        }
        return VectorCleanupResult.builder()
                .requestedCount(retired.size())
                .deletedCount(deleted)
                .warnings(warnings)
                .build();
    }

    public VectorCleanupResult cleanupPurgedDocumentResidue() {
        // PURGED 表示永久删除，因此 residual vector 必须清理。
        // TRASHED 不走这里，因为恢复文档时应尽量避免重新 embedding。
        List<String> warnings = new ArrayList<>();
        int deleted = 0;
        for (DocumentEntity document : documentRepository.findAll()) {
            if (document.getLifecycleStatus() != DocumentLifecycleStatus.PURGED) {
                continue;
            }
            Long collectionId = resolvePrimaryCollectionId(document.getId());
            if (collectionId == null) {
                vectorStore.deleteByDocumentId(document.getId());
                deleted++;
                continue;
            }
            VectorStoreOperationResult result = vectorStore.deleteByDocumentIdScoped(collectionId, document.getId());
            deleted += result.affectedCount();
            warnings.addAll(result.warnings());
        }
        return VectorCleanupResult.builder()
                .requestedCount(deleted)
                .deletedCount(deleted)
                .warnings(warnings)
                .build();
    }

    public VectorCleanupResult cleanupOrphanVectors(Long collectionId) {
        // Orphan cleanup 只清理 chunk 已不存在的向量。
        // 它必须限定 collection，防止一次操作跨知识库删除。
        if (collectionId == null) {
            throw new IllegalArgumentException("collectionId is required");
        }
        List<VectorStoreDocument> vectors = vectorStore.scanByFilter(VectorScanFilter.builder()
                .collectionId(collectionId)
                .build());
        Set<Long> existingChunkIds = documentChunkRepository.findAll().stream()
                .map(chunk -> chunk.getId())
                .collect(Collectors.toSet());
        int deleted = 0;
        List<String> warnings = new ArrayList<>();
        for (VectorStoreDocument vector : vectors) {
            if (vector.chunkId() != null && !existingChunkIds.contains(vector.chunkId())) {
                VectorStoreOperationResult result = vectorStore.deleteByVectorId(vector.vectorId());
                deleted += result.affectedCount();
                warnings.addAll(result.warnings());
            }
        }
        return VectorCleanupResult.builder()
                .requestedCount(vectors.size())
                .deletedCount(deleted)
                .warnings(warnings)
                .build();
    }

    public VectorCleanupResult cleanupPollutedVectors(Long collectionId) {
        // Polluted cleanup 处理 payload.collection_id 与目标 collection 不一致的向量。
        // 这是对 audit 结果的显式修复动作，不应由 audit 自动触发。
        if (collectionId == null) {
            throw new IllegalArgumentException("collectionId is required");
        }
        List<VectorStoreDocument> vectors = vectorStore.scanByFilter(VectorScanFilter.builder()
                .collectionId(collectionId)
                .build());
        int deleted = 0;
        List<String> warnings = new ArrayList<>();
        for (VectorStoreDocument vector : vectors) {
            if (vector.collectionId() != null && !Objects.equals(vector.collectionId(), collectionId)) {
                VectorStoreOperationResult result = vectorStore.deleteByVectorId(vector.vectorId());
                deleted += result.affectedCount();
                warnings.add("删除跨集合污染向量: " + vector.vectorId());
                warnings.addAll(result.warnings());
            }
        }
        return VectorCleanupResult.builder()
                .requestedCount(vectors.size())
                .deletedCount(deleted)
                .warnings(warnings)
                .build();
    }

    private Long resolvePrimaryCollectionId(Long documentId) {
        List<Object[]> rows = documentCollectionRepository.findCollectionSummariesByDocumentId(documentId);
        if (rows.isEmpty()) {
            return null;
        }
        Object id = rows.get(0)[0];
        return id instanceof Number number ? number.longValue() : null;
    }

    @Getter
    @Builder
    public static class VectorCleanupResult {
        private final int requestedCount;
        private final int deletedCount;
        private final int failedCount;
        private final int skippedCount;
        private final List<String> warnings;
        private final List<String> errors;
    }
}
