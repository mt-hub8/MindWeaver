package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.RagEvaluationRunEntity;
import com.tuoman.ai_task_orchestrator.kbhealth.RagEvaluationRunStatus;
import org.springframework.data.jpa.repository.JpaRepository;

import java.util.List;

public interface RagEvaluationRunRepository extends JpaRepository<RagEvaluationRunEntity, Long> {

    List<RagEvaluationRunEntity> findAllByOrderByCreatedAtDesc();

    List<RagEvaluationRunEntity> findByDatasetIdOrderByCreatedAtDesc(Long datasetId);

    List<RagEvaluationRunEntity> findByStatus(RagEvaluationRunStatus status);
}
