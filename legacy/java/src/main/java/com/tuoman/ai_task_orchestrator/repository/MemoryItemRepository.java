package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.MemoryItemEntity;
import com.tuoman.ai_task_orchestrator.enums.MemoryStatus;
import com.tuoman.ai_task_orchestrator.enums.MemoryType;
import org.springframework.data.jpa.repository.JpaRepository;

import java.util.List;

public interface MemoryItemRepository extends JpaRepository<MemoryItemEntity, Long> {

    List<MemoryItemEntity> findByMemoryKeyAndMemoryTypeAndStatus(
            String memoryKey,
            MemoryType memoryType,
            MemoryStatus status
    );

    List<MemoryItemEntity> findByAgentProfileIdAndStatusNotOrderByUpdatedAtDesc(
            Long agentProfileId,
            MemoryStatus status
    );

    List<MemoryItemEntity> findByStatusOrderByUpdatedAtDesc(MemoryStatus status);

    List<MemoryItemEntity> findAllByOrderByUpdatedAtDesc();

    long countByAgentProfileIdAndStatus(Long agentProfileId, MemoryStatus status);
}
