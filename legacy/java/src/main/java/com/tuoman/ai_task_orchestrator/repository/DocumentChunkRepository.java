package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.enums.ChunkStatus;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Modifying;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;

import java.util.List;

public interface DocumentChunkRepository extends JpaRepository<DocumentChunkEntity, Long> {

    List<DocumentChunkEntity> findByDocumentIdOrderByChunkIndexAsc(Long documentId);

    List<DocumentChunkEntity> findByDocumentIdAndGenerationOrderByChunkIndexAsc(Long documentId, Integer generation);

    List<DocumentChunkEntity> findByDocumentIdAndChunkStatusAndGenerationOrderByChunkIndexAsc(
            Long documentId,
            ChunkStatus chunkStatus,
            Integer generation
    );

    void deleteByDocumentId(Long documentId);

    void deleteByDocumentIdAndGeneration(Long documentId, Integer generation);

    int countByDocumentIdAndChunkStatusAndGeneration(Long documentId, ChunkStatus chunkStatus, Integer generation);

    @Query("""
            SELECT c.id FROM DocumentChunkEntity c
            JOIN DocumentEntity d ON d.id = c.documentId
            WHERE c.chunkStatus = com.tuoman.ai_task_orchestrator.enums.ChunkStatus.ACTIVE
              AND c.generation = d.currentGeneration
              AND d.lifecycleStatus = com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus.ACTIVE
            """)
    List<Long> findRetrievableChunkIds();

    @Modifying
    @Query("""
            UPDATE DocumentChunkEntity c
            SET c.chunkStatus = com.tuoman.ai_task_orchestrator.enums.ChunkStatus.SUPERSEDED
            WHERE c.documentId = :documentId
              AND c.generation < :generation
              AND c.chunkStatus = com.tuoman.ai_task_orchestrator.enums.ChunkStatus.ACTIVE
            """)
    int supersedeChunksBeforeGeneration(@Param("documentId") Long documentId, @Param("generation") Integer generation);
}
