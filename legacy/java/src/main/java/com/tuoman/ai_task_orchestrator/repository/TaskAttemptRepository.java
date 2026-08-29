package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.TaskAttemptEntity;
import com.tuoman.ai_task_orchestrator.enums.TaskAttemptStatus;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;

import java.util.List;
import java.util.Optional;

public interface TaskAttemptRepository extends JpaRepository<TaskAttemptEntity, Long> {

    @Query("select max(a.attemptNo) from TaskAttemptEntity a where a.taskId = :taskId")
    Optional<Integer> findMaxAttemptNoByTaskId(@Param("taskId") Long taskId);

    List<TaskAttemptEntity> findByTaskIdOrderByAttemptNoAsc(Long taskId);

    Optional<TaskAttemptEntity> findByTaskIdAndStatus(Long taskId, TaskAttemptStatus status);
}
