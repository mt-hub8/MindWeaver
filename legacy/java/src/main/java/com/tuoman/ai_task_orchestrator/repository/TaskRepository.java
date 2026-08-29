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

    @Modifying(flushAutomatically = true, clearAutomatically = true)
    @Query("""
            update TaskEntity t
            set t.nextRetryAt = :reservedUntil
            where t.id = :taskId
              and t.status = :status
              and t.nextRetryAt <= :now
            """)
    int reserveRetryDispatch(
            @Param("taskId") Long taskId,
            @Param("status") TaskStatus status,
            @Param("now") LocalDateTime now,
            @Param("reservedUntil") LocalDateTime reservedUntil
    );

    @Modifying(flushAutomatically = true, clearAutomatically = true)
    @Query("""
            update TaskEntity t
            set t.status = :targetStatus,
                t.resultContent = :resultContent,
                t.llmModel = :llmModel,
                t.renderedPrompt = :renderedPrompt,
                t.promptTemplateCode = :promptTemplateCode,
                t.nextRetryAt = null,
                t.errorMessage = null
            where t.id = :taskId
              and t.status = :runningStatus
            """)
    int markSucceededIfRunning(
            @Param("taskId") Long taskId,
            @Param("targetStatus") TaskStatus targetStatus,
            @Param("runningStatus") TaskStatus runningStatus,
            @Param("resultContent") String resultContent,
            @Param("llmModel") String llmModel,
            @Param("renderedPrompt") String renderedPrompt,
            @Param("promptTemplateCode") String promptTemplateCode
    );

    @Modifying(flushAutomatically = true, clearAutomatically = true)
    @Query("""
            update TaskEntity t
            set t.status = :targetStatus,
                t.errorMessage = :errorMessage,
                t.nextRetryAt = null
            where t.id = :taskId
              and t.status = :runningStatus
            """)
    int markFailedIfRunning(
            @Param("taskId") Long taskId,
            @Param("targetStatus") TaskStatus targetStatus,
            @Param("runningStatus") TaskStatus runningStatus,
            @Param("errorMessage") String errorMessage
    );

    @Modifying(flushAutomatically = true, clearAutomatically = true)
    @Query("""
            update TaskEntity t
            set t.status = :targetStatus,
                t.errorMessage = :errorMessage,
                t.nextRetryAt = null
            where t.id = :taskId
              and t.status = :runningStatus
            """)
    int markTimedOutIfRunning(
            @Param("taskId") Long taskId,
            @Param("targetStatus") TaskStatus targetStatus,
            @Param("runningStatus") TaskStatus runningStatus,
            @Param("errorMessage") String errorMessage
    );

    @Modifying(flushAutomatically = true, clearAutomatically = true)
    @Query("""
            update TaskEntity t
            set t.status = :targetStatus,
                t.retryCount = t.retryCount + 1,
                t.nextRetryAt = :nextRetryAt,
                t.errorMessage = :errorMessage
            where t.id = :taskId
              and t.status = :runningStatus
              and t.retryCount < t.maxRetry
            """)
    int markRetryPendingIfRunning(
            @Param("taskId") Long taskId,
            @Param("targetStatus") TaskStatus targetStatus,
            @Param("runningStatus") TaskStatus runningStatus,
            @Param("nextRetryAt") LocalDateTime nextRetryAt,
            @Param("errorMessage") String errorMessage
    );

    @Modifying(flushAutomatically = true, clearAutomatically = true)
    @Query("""
            update TaskEntity t
            set t.status = :targetStatus,
                t.nextRetryAt = null
            where t.id = :taskId
              and t.status in :allowedStatuses
            """)
    int markCancelledIfAllowed(
            @Param("taskId") Long taskId,
            @Param("targetStatus") TaskStatus targetStatus,
            @Param("allowedStatuses") Collection<TaskStatus> allowedStatuses
    );
}
