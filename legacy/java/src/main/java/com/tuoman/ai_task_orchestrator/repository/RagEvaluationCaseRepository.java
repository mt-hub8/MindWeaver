package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.RagEvaluationCaseEntity;
import org.springframework.data.jpa.repository.JpaRepository;

import java.util.List;
import java.util.Optional;

public interface RagEvaluationCaseRepository extends JpaRepository<RagEvaluationCaseEntity, Long> {

    List<RagEvaluationCaseEntity> findByDatasetIdAndEnabledTrueOrderByIdAsc(Long datasetId);

    List<RagEvaluationCaseEntity> findByDatasetIdOrderByIdAsc(Long datasetId);

    long countByDatasetIdAndEnabledTrue(Long datasetId);

    Optional<RagEvaluationCaseEntity> findByDatasetIdAndCaseId(Long datasetId, String caseId);
}
