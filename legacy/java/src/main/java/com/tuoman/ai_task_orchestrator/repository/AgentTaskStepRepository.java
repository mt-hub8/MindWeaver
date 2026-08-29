package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.AgentTaskStepEntity;
import org.springframework.data.jpa.repository.JpaRepository;

import java.util.List;
import java.util.Optional;

public interface AgentTaskStepRepository extends JpaRepository<AgentTaskStepEntity, Long> {

    List<AgentTaskStepEntity> findByTaskIdOrderByStepOrderAsc(Long taskId);

    boolean existsByTaskId(Long taskId);

    Optional<AgentTaskStepEntity> findByTaskIdAndStepOrder(Long taskId, Integer stepOrder);
}
