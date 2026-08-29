package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.TaskOutputChunkEntity;
import org.springframework.data.jpa.repository.JpaRepository;

import java.util.List;

public interface TaskOutputChunkRepository extends JpaRepository<TaskOutputChunkEntity, Long> {

    List<TaskOutputChunkEntity> findByTaskIdOrderByChunkIndexAsc(Long taskId);

    void deleteByTaskId(Long taskId);
}
