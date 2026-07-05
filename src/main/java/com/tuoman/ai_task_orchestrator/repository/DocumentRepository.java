package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;

import java.time.LocalDateTime;
import java.util.List;

public interface DocumentRepository extends JpaRepository<DocumentEntity, Long> {

    List<DocumentEntity> findAllByLifecycleStatusOrderByCreatedAtDesc(DocumentLifecycleStatus lifecycleStatus);

    List<DocumentEntity> findByLifecycleStatusOrderByTrashedAtDesc(DocumentLifecycleStatus lifecycleStatus);

    List<DocumentEntity> findByLifecycleStatusAndPurgeAfterBefore(
            DocumentLifecycleStatus lifecycleStatus,
            LocalDateTime purgeAfter
    );

    @Query("SELECT d.id FROM DocumentEntity d WHERE d.lifecycleStatus = :status")
    List<Long> findIdsByLifecycleStatus(@Param("status") DocumentLifecycleStatus status);

    @Query("SELECT d.id FROM DocumentEntity d WHERE d.lifecycleStatus <> com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus.ACTIVE")
    List<Long> findNonRetrievableDocumentIds();

    @Query("SELECT COALESCE(SUM(d.fileSize), 0) FROM DocumentEntity d WHERE d.lifecycleStatus = com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus.ACTIVE")
    Long sumActiveFileSizeBytes();

    @Query("SELECT COALESCE(SUM(LENGTH(d.sourceText)), 0) FROM DocumentEntity d WHERE d.lifecycleStatus = com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus.ACTIVE AND d.sourceText IS NOT NULL")
    Long sumActiveSourceTextBytes();
}
