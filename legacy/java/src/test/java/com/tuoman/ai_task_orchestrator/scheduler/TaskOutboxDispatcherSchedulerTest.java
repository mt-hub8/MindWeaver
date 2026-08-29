package com.tuoman.ai_task_orchestrator.scheduler;

import com.tuoman.ai_task_orchestrator.entity.TaskOutboxEntity;
import com.tuoman.ai_task_orchestrator.enums.TaskOutboxStatus;
import com.tuoman.ai_task_orchestrator.repository.TaskOutboxRepository;
import com.tuoman.ai_task_orchestrator.service.TaskOutboxDispatchService;
import org.junit.jupiter.api.Test;

import java.time.LocalDateTime;
import java.util.List;
import java.util.Optional;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class TaskOutboxDispatcherSchedulerTest {

    private final TaskOutboxRepository taskOutboxRepository = mock(TaskOutboxRepository.class);

    private final TaskOutboxDispatchService taskOutboxDispatchService = mock(TaskOutboxDispatchService.class);

    private final TaskOutboxDispatcherScheduler scheduler = new TaskOutboxDispatcherScheduler(
            taskOutboxRepository,
            taskOutboxDispatchService
    );

    @Test
    void dueOutboxShouldBeSentAndMarkedSent() {
        TaskOutboxEntity outbox = outbox(1L, 101L);
        when(taskOutboxDispatchService.claimDueOutbox(1L)).thenReturn(Optional.of(outbox));
        when(taskOutboxDispatchService.sendOutboxMessage(outbox)).thenReturn(101L);

        scheduler.dispatchSingleOutbox(1L);

        verify(taskOutboxDispatchService).sendOutboxMessage(outbox);
        verify(taskOutboxDispatchService).markSent(1L);
        verify(taskOutboxDispatchService, never()).markFailed(eq(1L), any());
    }

    @Test
    void sendFailureShouldMarkOutboxFailed() {
        TaskOutboxEntity outbox = outbox(2L, 102L);
        RuntimeException failure = new RuntimeException("rabbit down");
        when(taskOutboxDispatchService.claimDueOutbox(2L)).thenReturn(Optional.of(outbox));
        when(taskOutboxDispatchService.sendOutboxMessage(outbox)).thenThrow(failure);

        scheduler.dispatchSingleOutbox(2L);

        verify(taskOutboxDispatchService).markFailed(eq(2L), eq(failure));
        verify(taskOutboxDispatchService, never()).markSent(2L);
    }

    @Test
    void claimFailureShouldNotSendMessage() {
        when(taskOutboxDispatchService.claimDueOutbox(3L)).thenReturn(Optional.empty());

        scheduler.dispatchSingleOutbox(3L);

        verify(taskOutboxDispatchService, never()).sendOutboxMessage(any());
        verify(taskOutboxDispatchService, never()).markSent(any());
        verify(taskOutboxDispatchService, never()).markFailed(any(), any());
    }

    @Test
    void dispatchDueOutboxesShouldProcessBatchIncludingStaleProcessingCandidates() {
        when(taskOutboxRepository.findDueOutboxes(
                eq(List.of(TaskOutboxStatus.PENDING, TaskOutboxStatus.FAILED)),
                eq(TaskOutboxStatus.PROCESSING),
                any(LocalDateTime.class),
                any(LocalDateTime.class),
                any()
        )).thenReturn(List.of(outbox(4L, 104L)));
        when(taskOutboxDispatchService.claimDueOutbox(4L)).thenReturn(Optional.empty());

        scheduler.dispatchDueOutboxes();

        verify(taskOutboxDispatchService).claimDueOutbox(4L);
    }

    private TaskOutboxEntity outbox(Long id, Long taskId) {
        TaskOutboxEntity outbox = new TaskOutboxEntity();
        outbox.setId(id);
        outbox.setAggregateType("TASK");
        outbox.setAggregateId(taskId);
        outbox.setEventType("TASK_DISPATCH_REQUESTED");
        outbox.setPayload("{\"taskId\":" + taskId + "}");
        outbox.setStatus(TaskOutboxStatus.PENDING);
        outbox.setRetryCount(0);
        return outbox;
    }
}
