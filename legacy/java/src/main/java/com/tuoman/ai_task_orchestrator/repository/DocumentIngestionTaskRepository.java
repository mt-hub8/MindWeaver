package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.DocumentIngestionTaskEntity;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStatus;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskType;
import org.springframework.data.jpa.repository.JpaRepository;

import java.time.LocalDateTime;
import java.util.Collection;
import java.util.List;

public interface DocumentIngestionTaskRepository extends JpaRepository<DocumentIngestionTaskEntity, Long> {

    List<DocumentIngestionTaskEntity> findTop20ByOrderByCreatedAtDesc();

    List<DocumentIngestionTaskEntity> findByCreatedAtGreaterThanEqual(LocalDateTime createdAt);

    boolean existsByDocumentIdAndTaskTypeAndStatusIn(
            Long documentId,
            IngestionTaskType taskType,
            Collection<IngestionTaskStatus> statuses
    );
}
