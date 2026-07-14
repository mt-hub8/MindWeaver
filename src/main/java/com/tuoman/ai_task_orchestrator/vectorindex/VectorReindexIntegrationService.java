package com.tuoman.ai_task_orchestrator.vectorindex;

import com.tuoman.ai_task_orchestrator.embedding.EmbeddingProvider;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.VectorGenerationStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentCollectionRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.List;

/**
 * 文档 reindex 与 V16 generation 的集成服务。
 *
 * ingestion handler 在 REINDEX task 的开始、完成、失败三个阶段调用这里，
 * 把文档级重新索引映射为 BUILDING -> ACTIVE/FAILED 的向量索引状态机。
 */
@Service
@RequiredArgsConstructor
public class VectorReindexIntegrationService {

    private final VectorGenerationService vectorGenerationService;

    private final VectorCleanupService vectorCleanupService;

    private final DocumentCollectionRepository documentCollectionRepository;

    private final EmbeddingProvider embeddingProvider;

    @Transactional
    public void onReindexStarted(Long documentId, int targetGeneration) {
        // 新 generation 先进入 BUILDING，允许写入但不参与检索。
        // 这样旧 ACTIVE generation 在重建期间仍可稳定服务。
        Long collectionId = resolvePrimaryCollectionId(documentId);
        vectorGenerationService.beginBuildingGeneration(
                collectionId,
                documentId,
                (long) targetGeneration,
                embeddingProvider.model(),
                embeddingProvider.dimension()
        );
    }

    @Transactional
    public void onReindexCompleted(Long documentId, int targetGeneration) {
        // 只有文档 chunk、embedding、vector write 全部完成后才激活新 generation。
        // 旧 RETIRED generation 可以在确认新索引可用后清理，避免新旧混召回。
        Long collectionId = resolvePrimaryCollectionId(documentId);
        vectorGenerationService.activateGeneration(collectionId, documentId, (long) targetGeneration);
        if (collectionId != null) {
            List<com.tuoman.ai_task_orchestrator.entity.VectorIndexGenerationEntity> retired =
                    vectorGenerationService.findRetiredGenerations(documentId);
            for (var generation : retired) {
                vectorCleanupService.cleanupGenerationVectors(collectionId, generation.getGeneration());
            }
        }
    }

    @Transactional
    public void onReindexFailed(Long documentId, int targetGeneration, String message) {
        // 失败只污染 BUILDING generation，不影响旧 ACTIVE generation。
        // 这保证 reindex 失败不是线上检索失败。
        vectorGenerationService.markGenerationFailed(documentId, (long) targetGeneration, message);
    }

    private Long resolvePrimaryCollectionId(Long documentId) {
        List<Object[]> rows = documentCollectionRepository.findCollectionSummariesByDocumentId(documentId);
        if (rows.isEmpty()) {
            return null;
        }
        Object id = rows.get(0)[0];
        return id instanceof Number number ? number.longValue() : null;
    }
}
