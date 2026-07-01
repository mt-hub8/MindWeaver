package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.TaskOutboxEntity;
import com.tuoman.ai_task_orchestrator.enums.TaskOutboxStatus;
import org.springframework.data.domain.Pageable;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Modifying;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;

import java.time.LocalDateTime;
import java.util.Collection;
import java.util.List;

public interface TaskOutboxRepository extends JpaRepository<TaskOutboxEntity, Long> {

    @Query("""
            select o
            from TaskOutboxEntity o
            where (
                    o.status in :statuses
                    and (o.nextRetryAt is null or o.nextRetryAt <= :now)
                  )
               or (
                    o.status = :processingStatus
                    and o.lockedAt < :staleThreshold
                  )
            order by o.createdAt asc
            """)
    List<TaskOutboxEntity> findDueOutboxes(
            @Param("statuses") Collection<TaskOutboxStatus> statuses,
            @Param("processingStatus") TaskOutboxStatus processingStatus,
            @Param("now") LocalDateTime now,
            @Param("staleThreshold") LocalDateTime staleThreshold,
            Pageable pageable
    );

    @Modifying(flushAutomatically = true, clearAutomatically = true)
    @Query("""
            update TaskOutboxEntity o
            set o.status = :processingStatus,
                o.lockedBy = :lockedBy,
                o.lockedAt = :lockedAt,
                o.updatedAt = :lockedAt
            where o.id = :outboxId
              and (
                    (o.status in :claimableStatuses and (o.nextRetryAt is null or o.nextRetryAt <= :now))
                    or (o.status = :processingStatus and o.lockedAt < :staleThreshold)
                  )
            """)
    int claimOutbox(
            @Param("outboxId") Long outboxId,
            @Param("processingStatus") TaskOutboxStatus processingStatus,
            @Param("claimableStatuses") Collection<TaskOutboxStatus> claimableStatuses,
            @Param("lockedBy") String lockedBy,
            @Param("lockedAt") LocalDateTime lockedAt,
            @Param("now") LocalDateTime now,
            @Param("staleThreshold") LocalDateTime staleThreshold
    );

    @Modifying(flushAutomatically = true, clearAutomatically = true)
    @Query("""
            update TaskOutboxEntity o
            set o.status = :sentStatus,
                o.lockedBy = null,
                o.lockedAt = null,
                o.lastErrorMessage = null,
                o.updatedAt = :now
            where o.id = :outboxId
              and o.status = :processingStatus
            """)
    int markSent(
            @Param("outboxId") Long outboxId,
            @Param("processingStatus") TaskOutboxStatus processingStatus,
            @Param("sentStatus") TaskOutboxStatus sentStatus,
            @Param("now") LocalDateTime now
    );

    @Modifying(flushAutomatically = true, clearAutomatically = true)
    @Query("""
            update TaskOutboxEntity o
            set o.status = :failedStatus,
                o.retryCount = o.retryCount + 1,
                o.nextRetryAt = :nextRetryAt,
                o.lockedBy = null,
                o.lockedAt = null,
                o.lastErrorMessage = :lastErrorMessage,
                o.updatedAt = :now
            where o.id = :outboxId
              and o.status = :processingStatus
            """)
    int markFailed(
            @Param("outboxId") Long outboxId,
            @Param("processingStatus") TaskOutboxStatus processingStatus,
            @Param("failedStatus") TaskOutboxStatus failedStatus,
            @Param("nextRetryAt") LocalDateTime nextRetryAt,
            @Param("lastErrorMessage") String lastErrorMessage,
            @Param("now") LocalDateTime now
    );
}
