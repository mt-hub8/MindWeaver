package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.RagEvaluationDatasetEntity;
import com.tuoman.ai_task_orchestrator.kbhealth.RagEvaluationDatasetStatus;
import org.springframework.data.jpa.repository.JpaRepository;

import java.util.List;

public interface RagEvaluationDatasetRepository extends JpaRepository<RagEvaluationDatasetEntity, Long> {

    List<RagEvaluationDatasetEntity> findAllByOrderByCreatedAtDesc();

    List<RagEvaluationDatasetEntity> findByStatusOrderByCreatedAtDesc(RagEvaluationDatasetStatus status);
}
