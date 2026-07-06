package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.UploadBatchEntity;
import org.springframework.data.jpa.repository.JpaRepository;

import java.util.List;

public interface UploadBatchRepository extends JpaRepository<UploadBatchEntity, Long> {

    List<UploadBatchEntity> findAllByOrderByCreatedAtDesc();
}
