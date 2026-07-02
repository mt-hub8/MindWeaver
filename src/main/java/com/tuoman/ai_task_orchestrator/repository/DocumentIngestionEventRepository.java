package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.DocumentIngestionEventEntity;
import org.springframework.data.jpa.repository.JpaRepository;

import java.util.List;
import java.util.Optional;

public interface DocumentIngestionEventRepository extends JpaRepository<DocumentIngestionEventEntity, Long> {

    List<DocumentIngestionEventEntity> findByTaskIdOrderByCreatedAtAsc(Long taskId);

    Optional<DocumentIngestionEventEntity> findTopByTaskIdOrderByCreatedAtDesc(Long taskId);
}
