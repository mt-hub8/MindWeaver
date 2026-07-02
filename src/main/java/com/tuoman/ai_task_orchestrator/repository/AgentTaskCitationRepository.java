package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.AgentTaskCitationEntity;
import org.springframework.data.jpa.repository.JpaRepository;

import java.util.List;

public interface AgentTaskCitationRepository extends JpaRepository<AgentTaskCitationEntity, Long> {

    List<AgentTaskCitationEntity> findByTaskIdOrderBySourceIndexAsc(Long taskId);

    void deleteByTaskId(Long taskId);
}
