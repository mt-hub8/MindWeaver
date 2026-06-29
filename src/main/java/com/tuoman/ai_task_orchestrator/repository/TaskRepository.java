package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.TaskEntity;
import com.tuoman.ai_task_orchestrator.enums.TaskStatus;
import org.springframework.data.jpa.repository.JpaRepository;

import java.time.LocalDateTime;
import java.util.List;

public interface TaskRepository extends JpaRepository<TaskEntity, Long> {

    List<TaskEntity> findTop20ByStatusAndNextRetryAtLessThanEqualOrderByNextRetryAtAsc(
            TaskStatus status,
            LocalDateTime now
    );
}
