package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.RetrievalReindexEventEntity;
import org.springframework.data.jpa.repository.JpaRepository;

public interface RetrievalReindexEventRepository extends JpaRepository<RetrievalReindexEventEntity, Long> {
}
