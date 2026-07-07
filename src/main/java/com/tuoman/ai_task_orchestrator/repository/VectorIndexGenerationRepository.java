package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.VectorIndexGenerationEntity;
import com.tuoman.ai_task_orchestrator.enums.VectorGenerationStatus;
import org.springframework.data.jpa.repository.JpaRepository;

import java.util.List;
import java.util.Optional;

public interface VectorIndexGenerationRepository extends JpaRepository<VectorIndexGenerationEntity, Long> {

    Optional<VectorIndexGenerationEntity> findFirstByDocumentIdAndStatusOrderByGenerationDesc(
            Long documentId,
            VectorGenerationStatus status
    );

    Optional<VectorIndexGenerationEntity> findFirstByCollectionIdAndDocumentIdIsNullAndStatusOrderByGenerationDesc(
            Long collectionId,
            VectorGenerationStatus status
    );

    List<VectorIndexGenerationEntity> findByDocumentIdAndStatus(Long documentId, VectorGenerationStatus status);

    List<VectorIndexGenerationEntity> findByCollectionIdAndDocumentIdIsNullAndStatus(
            Long collectionId,
            VectorGenerationStatus status
    );

    Optional<VectorIndexGenerationEntity> findByDocumentIdAndGeneration(Long documentId, Long generation);

    Optional<VectorIndexGenerationEntity> findByCollectionIdAndDocumentIdIsNullAndGeneration(
            Long collectionId,
            Long generation
    );
}
