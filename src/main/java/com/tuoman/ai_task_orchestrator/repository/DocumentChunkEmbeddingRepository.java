package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEmbeddingEntity;
import org.springframework.data.jpa.repository.JpaRepository;

import java.util.List;
import java.util.Optional;

public interface DocumentChunkEmbeddingRepository extends JpaRepository<DocumentChunkEmbeddingEntity, Long> {

    List<DocumentChunkEmbeddingEntity> findByEmbeddingProviderAndEmbeddingModel(
            String embeddingProvider,
            String embeddingModel
    );

    List<DocumentChunkEmbeddingEntity> findByDocumentIdAndEmbeddingProviderAndEmbeddingModel(
            Long documentId,
            String embeddingProvider,
            String embeddingModel
    );

    void deleteByDocumentIdAndEmbeddingProviderAndEmbeddingModel(
            Long documentId,
            String embeddingProvider,
            String embeddingModel
    );

    Optional<DocumentChunkEmbeddingEntity> findByDocumentChunkIdAndEmbeddingProviderAndEmbeddingModel(
            Long documentChunkId,
            String embeddingProvider,
            String embeddingModel
    );
}
