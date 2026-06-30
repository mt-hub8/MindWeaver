package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.TaskEntity;
import com.tuoman.ai_task_orchestrator.enums.TaskStatus;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Modifying;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;

import java.time.LocalDateTime;
import java.util.Collection;
import java.util.List;

public interface TaskRepository extends JpaRepository<TaskEntity, Long> {

    List<TaskEntity> findTop20ByStatusAndNextRetryAtLessThanEqualOrderByNextRetryAtAsc(
            TaskStatus status,
            LocalDateTime now
    );

    List<TaskEntity> findTop20ByStatusAndTimeoutAtLessThanEqualOrderByTimeoutAtAsc(
            TaskStatus status,
            LocalDateTime now
    );

    @Modifying(flushAutomatically = true, clearAutomatically = true)
    @Query("""
            update TaskEntity t
            set t.status = :targetStatus,
                t.timeoutAt = :timeoutAt
            where t.id = :taskId
              and t.status in :allowedStatuses
            """)
    int claimTaskForExecution(
            @Param("taskId") Long taskId,
            @Param("targetStatus") TaskStatus targetStatus,
            @Param("timeoutAt") LocalDateTime timeoutAt,
            @Param("allowedStatuses") Collection<TaskStatus> allowedStatuses
    );
}
