package com.tuoman.ai_task_orchestrator.vectorindex;

import com.tuoman.ai_task_orchestrator.embedding.EmbeddingProvider;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.VectorGenerationStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentCollectionRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.List;

@Service
@RequiredArgsConstructor
public class VectorReindexIntegrationService {

    private final VectorGenerationService vectorGenerationService;

    private final VectorCleanupService vectorCleanupService;

    private final DocumentCollectionRepository documentCollectionRepository;

    private final EmbeddingProvider embeddingProvider;

    @Transactional
    public void onReindexStarted(Long documentId, int targetGeneration) {
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
