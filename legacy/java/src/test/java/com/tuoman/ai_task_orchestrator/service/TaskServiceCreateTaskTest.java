package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.CreateTaskRequest;
import com.tuoman.ai_task_orchestrator.entity.TaskEntity;
import com.tuoman.ai_task_orchestrator.entity.TaskEventEntity;
import com.tuoman.ai_task_orchestrator.enums.TaskStatus;
import com.tuoman.ai_task_orchestrator.mq.TaskDispatchProducer;
import com.tuoman.ai_task_orchestrator.repository.TaskEventRepository;
import com.tuoman.ai_task_orchestrator.repository.TaskRepository;
import com.tuoman.ai_task_orchestrator.state.TaskStateMachine;
import org.junit.jupiter.api.Test;

import java.lang.reflect.Field;
import java.util.Arrays;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class TaskServiceCreateTaskTest {

    private final TaskRepository taskRepository = mock(TaskRepository.class);

    private final TaskEventRepository taskEventRepository = mock(TaskEventRepository.class);

    private final TaskStateMachine taskStateMachine = mock(TaskStateMachine.class);

    private final TaskOutboxService taskOutboxService = mock(TaskOutboxService.class);

    private final TaskAttemptService taskAttemptService = mock(TaskAttemptService.class);

    private final TaskOutputChunkService taskOutputChunkService = mock(TaskOutputChunkService.class);

    private final TaskService taskService = new TaskService(
            taskRepository,
            taskEventRepository,
            taskStateMachine,
            taskOutboxService,
            taskAttemptService,
            taskOutputChunkService
    );

    @Test
    void createTaskShouldSaveTaskEventAndOutboxWithoutDirectProducerDependency() {
        CreateTaskRequest request = new CreateTaskRequest();
        request.setPrompt("normal task");
        request.setModel("mock-fast");

        when(taskRepository.save(any(TaskEntity.class))).thenAnswer(invocation -> {
            TaskEntity task = invocation.getArgument(0);
            task.setId(1L);
            return task;
        });

        taskService.createTask(request);

        verify(taskRepository).save(any(TaskEntity.class));
        verify(taskEventRepository).save(any(TaskEventEntity.class));
        verify(taskOutboxService).createTaskDispatchOutbox(1L);

        boolean hasProducerDependency = Arrays.stream(TaskService.class.getDeclaredFields())
                .map(Field::getType)
                .anyMatch(TaskDispatchProducer.class::equals);
        assertThat(hasProducerDependency).isFalse();
    }

    @Test
    void createTaskShouldKeepPendingResponseContract() {
        CreateTaskRequest request = new CreateTaskRequest();
        request.setPrompt("normal task");

        when(taskRepository.save(any(TaskEntity.class))).thenAnswer(invocation -> {
            TaskEntity task = invocation.getArgument(0);
            task.setId(2L);
            return task;
        });

        var response = taskService.createTask(request);

        assertThat(response.getTaskId()).isEqualTo(2L);
        assertThat(response.getStatus()).isEqualTo(TaskStatus.PENDING);
    }
}
