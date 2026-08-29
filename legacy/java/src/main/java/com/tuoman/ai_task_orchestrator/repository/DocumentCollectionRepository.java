package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.DocumentCollectionEntity;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;

import java.util.List;
import java.util.Optional;

public interface DocumentCollectionRepository extends JpaRepository<DocumentCollectionEntity, Long> {

    boolean existsByCollectionIdAndDocumentId(Long collectionId, Long documentId);

    Optional<DocumentCollectionEntity> findByCollectionIdAndDocumentId(Long collectionId, Long documentId);

    void deleteByCollectionIdAndDocumentId(Long collectionId, Long documentId);

    void deleteByDocumentId(Long documentId);

    List<DocumentCollectionEntity> findByCollectionIdOrderByCreatedAtAsc(Long collectionId);

    List<DocumentCollectionEntity> findByDocumentIdOrderByCreatedAtAsc(Long documentId);

    int countByCollectionId(Long collectionId);

    @Query("""
            SELECT COUNT(dc) FROM DocumentCollectionEntity dc
            JOIN DocumentEntity d ON d.id = dc.documentId
            WHERE dc.collectionId = :collectionId
              AND d.lifecycleStatus = com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus.ACTIVE
            """)
    int countActiveDocumentsByCollectionId(@Param("collectionId") Long collectionId);

    @Query("""
            SELECT dc.documentId FROM DocumentCollectionEntity dc
            WHERE dc.collectionId = :collectionId
            """)
    List<Long> findDocumentIdsByCollectionId(@Param("collectionId") Long collectionId);

    @Query("""
            SELECT DISTINCT d.id FROM DocumentEntity d
            JOIN DocumentCollectionEntity dc ON dc.documentId = d.id
            WHERE dc.collectionId = :collectionId
              AND d.lifecycleStatus = com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus.ACTIVE
              AND d.status = com.tuoman.ai_task_orchestrator.enums.DocumentStatus.READY
              AND EXISTS (
                  SELECT 1 FROM DocumentChunkEntity c
                  WHERE c.documentId = d.id
                    AND c.chunkStatus = com.tuoman.ai_task_orchestrator.enums.ChunkStatus.ACTIVE
                    AND c.generation = d.currentGeneration
              )
            """)
    List<Long> findAskableDocumentIdsByCollectionId(@Param("collectionId") Long collectionId);

    @Query("""
            SELECT kc.id, kc.name FROM KnowledgeCollectionEntity kc
            JOIN DocumentCollectionEntity dc ON dc.collectionId = kc.id
            WHERE dc.documentId = :documentId
            ORDER BY kc.name ASC
            """)
    List<Object[]> findCollectionSummariesByDocumentId(@Param("documentId") Long documentId);
}
