package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.DocumentIngestionTaskEntity;
import org.springframework.data.jpa.repository.JpaRepository;

import java.util.List;

public interface DocumentIngestionTaskRepository extends JpaRepository<DocumentIngestionTaskEntity, Long> {

    List<DocumentIngestionTaskEntity> findTop20ByOrderByCreatedAtDesc();
}
