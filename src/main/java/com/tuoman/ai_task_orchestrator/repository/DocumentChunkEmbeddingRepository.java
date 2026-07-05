package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEmbeddingEntity;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Query;

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

    void deleteByDocumentId(Long documentId);

    Optional<DocumentChunkEmbeddingEntity> findByDocumentChunkIdAndEmbeddingProviderAndEmbeddingModel(
            Long documentChunkId,
            String embeddingProvider,
            String embeddingModel
    );

    @Query("SELECT DISTINCT e.vectorDimension FROM DocumentChunkEmbeddingEntity e")
    List<Integer> findDistinctVectorDimensions();
}
