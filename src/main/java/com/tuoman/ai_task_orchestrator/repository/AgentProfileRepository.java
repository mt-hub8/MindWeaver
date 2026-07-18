package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.AgentProfileEntity;
import org.springframework.data.jpa.repository.JpaRepository;

import java.util.List;
import java.util.Optional;

public interface AgentProfileRepository extends JpaRepository<AgentProfileEntity, Long> {

    Optional<AgentProfileEntity> findByAgentKey(String agentKey);

    boolean existsByAgentKey(String agentKey);

    List<AgentProfileEntity> findAllByOrderByCreatedAtAsc();
}
