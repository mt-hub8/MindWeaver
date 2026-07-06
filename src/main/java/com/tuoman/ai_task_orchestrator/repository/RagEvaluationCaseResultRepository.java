package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.RagEvaluationCaseResultEntity;
import org.springframework.data.jpa.repository.JpaRepository;

import java.util.List;

public interface RagEvaluationCaseResultRepository extends JpaRepository<RagEvaluationCaseResultEntity, Long> {

    List<RagEvaluationCaseResultEntity> findByRunIdOrderByIdAsc(Long runId);
}
