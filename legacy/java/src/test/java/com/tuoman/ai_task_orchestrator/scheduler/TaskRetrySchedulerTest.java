package com.tuoman.ai_task_orchestrator.scheduler;

import com.tuoman.ai_task_orchestrator.entity.TaskEntity;
import com.tuoman.ai_task_orchestrator.enums.TaskStatus;
import com.tuoman.ai_task_orchestrator.repository.TaskRepository;
import com.tuoman.ai_task_orchestrator.service.TaskOutboxService;
import org.junit.jupiter.api.Test;

import java.time.LocalDateTime;
import java.util.List;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class TaskRetrySchedulerTest {

    private final TaskRepository taskRepository = mock(TaskRepository.class);

    private final TaskOutboxService taskOutboxService = mock(TaskOutboxService.class);

    private final TaskRetryScheduler scheduler = new TaskRetryScheduler(taskRepository, taskOutboxService);

    @Test
    void dueRetryTaskShouldBeReservedAndWrittenToOutbox() {
        TaskEntity task = new TaskEntity();
        task.setId(10L);
        task.setStatus(TaskStatus.RETRY_PENDING);
        task.setNextRetryAt(LocalDateTime.now().minusSeconds(1));
        when(taskRepository.findTop20ByStatusAndNextRetryAtLessThanEqualOrderByNextRetryAtAsc(
                eq(TaskStatus.RETRY_PENDING),
                any(LocalDateTime.class)
        )).thenReturn(List.of(task));
        when(taskRepository.reserveRetryDispatch(
                eq(10L),
                eq(TaskStatus.RETRY_PENDING),
                any(LocalDateTime.class),
                any(LocalDateTime.class)
        )).thenReturn(1);

        scheduler.dispatchRetryTasks();

        verify(taskOutboxService).createTaskDispatchOutbox(10L);
    }

    @Test
    void reserveFailureShouldNotWriteOutbox() {
        when(taskRepository.reserveRetryDispatch(
                eq(11L),
                eq(TaskStatus.RETRY_PENDING),
                any(LocalDateTime.class),
                any(LocalDateTime.class)
        )).thenReturn(0);

        scheduler.reserveRetryTaskAndCreateOutbox(11L);

        verify(taskOutboxService, never()).createTaskDispatchOutbox(11L);
    }

    @Test
    void noDueRetryTaskShouldDoNothing() {
        when(taskRepository.findTop20ByStatusAndNextRetryAtLessThanEqualOrderByNextRetryAtAsc(
                eq(TaskStatus.RETRY_PENDING),
                any(LocalDateTime.class)
        )).thenReturn(List.of());

        scheduler.dispatchRetryTasks();

        verify(taskOutboxService, never()).createTaskDispatchOutbox(any());
    }
}
