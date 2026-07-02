package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.AgentTaskEntity;
import org.springframework.data.jpa.repository.JpaRepository;

import java.util.List;

public interface AgentTaskRepository extends JpaRepository<AgentTaskEntity, Long> {

    List<AgentTaskEntity> findTop20ByOrderByCreatedAtDesc();
}
