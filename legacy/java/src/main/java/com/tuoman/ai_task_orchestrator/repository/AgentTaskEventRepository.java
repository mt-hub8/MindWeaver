package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.AgentTaskEventEntity;
import org.springframework.data.jpa.repository.JpaRepository;

import java.util.List;

public interface AgentTaskEventRepository extends JpaRepository<AgentTaskEventEntity, Long> {

    List<AgentTaskEventEntity> findByTaskIdOrderByCreatedAtAsc(Long taskId);
}
