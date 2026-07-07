package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.VectorAuditRunEntity;
import org.springframework.data.jpa.repository.JpaRepository;

import java.util.List;

public interface VectorAuditRunRepository extends JpaRepository<VectorAuditRunEntity, Long> {

    List<VectorAuditRunEntity> findAllByOrderByCreatedAtDesc();
}
