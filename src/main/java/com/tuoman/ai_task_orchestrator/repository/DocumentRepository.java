package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;

import java.util.List;

public interface DocumentRepository extends JpaRepository<DocumentEntity, Long> {

    List<DocumentEntity> findAllByOrderByCreatedAtDesc();

    @Query("SELECT d.id FROM DocumentEntity d WHERE d.lifecycleStatus = :status")
    List<Long> findIdsByLifecycleStatus(@Param("status") DocumentLifecycleStatus status);
}
