package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.entity.TaskEntity;
import com.tuoman.ai_task_orchestrator.enums.TaskStatus;
import com.tuoman.ai_task_orchestrator.llm.LlmResponse;
import com.tuoman.ai_task_orchestrator.repository.TaskRepository;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.transaction.annotation.Transactional;

import java.time.LocalDateTime;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest(properties = "spring.task.scheduling.enabled=false")
@Transactional
class TaskServiceFinalizationTest {

    private static final String TIMEOUT_ERROR_MESSAGE = "任务执行超时";

    @Autowired
    private TaskService taskService;

    @Autowired
    private TaskRepository taskRepository;

    @Autowired
    private TaskAttemptService taskAttemptService;

    @Test
    void successFinalizationShouldSucceedForRunningTask() {
        TaskEntity task = saveRunningTask();
        var attempt = taskAttemptService.createRunningAttempt(task.getId());

        boolean finalized = taskService.finalizeSuccessfulExecution(
                task.getId(),
                attempt.getId(),
                successResponse(),
                "rendered",
                "default_task_prompt"
        );

        assertThat(finalized).isTrue();
        TaskEntity saved = taskRepository.findById(task.getId()).orElseThrow();
        assertThat(saved.getStatus()).isEqualTo(TaskStatus.SUCCESS);
        assertThat(saved.getResultContent()).isEqualTo("content");
    }

    @Test
    void successFinalizationShouldBeRejectedWhenTaskAlreadyFailed() {
        TaskEntity task = saveRunningTask();
        var attempt = taskAttemptService.createRunningAttempt(task.getId());
        task.setStatus(TaskStatus.FAILED);
        task.setErrorMessage("already failed");
        taskRepository.save(task);

        boolean finalized = taskService.finalizeSuccessfulExecution(
                task.getId(),
                attempt.getId(),
                successResponse(),
                "rendered",
                "default_task_prompt"
        );

        assertThat(finalized).isFalse();
        assertThat(taskRepository.findById(task.getId()).orElseThrow().getStatus()).isEqualTo(TaskStatus.FAILED);
    }

    @Test
    void successFinalizationShouldBeRejectedWhenTaskAlreadyTimedOut() {
        TaskEntity task = saveRunningTask();
        var attempt = taskAttemptService.createRunningAttempt(task.getId());
        task.setStatus(TaskStatus.FAILED);
        task.setErrorMessage(TIMEOUT_ERROR_MESSAGE);
        taskRepository.save(task);

        boolean finalized = taskService.finalizeSuccessfulExecution(
                task.getId(),
                attempt.getId(),
                successResponse(),
                "rendered",
                "default_task_prompt"
        );

        assertThat(finalized).isFalse();
        assertThat(taskRepository.findById(task.getId()).orElseThrow().getErrorMessage()).isEqualTo(TIMEOUT_ERROR_MESSAGE);
    }

    @Test
    void failedFinalizationShouldNotOverwriteSuccess() {
        TaskEntity task = saveRunningTask();
        task.setStatus(TaskStatus.SUCCESS);
        task.setResultContent("done");
        taskRepository.save(task);

        boolean updated = taskService.tryMarkTaskFailed(task.getId(), "late failure");

        assertThat(updated).isFalse();
        assertThat(taskRepository.findById(task.getId()).orElseThrow().getStatus()).isEqualTo(TaskStatus.SUCCESS);
    }

    @Test
    void timeoutFinalizationShouldNotOverwriteSuccess() {
        TaskEntity task = saveRunningTask();
        task.setStatus(TaskStatus.SUCCESS);
        task.setResultContent("done");
        taskRepository.save(task);

        boolean updated = taskService.tryMarkTaskTimedOut(task.getId());

        assertThat(updated).isFalse();
        assertThat(taskRepository.findById(task.getId()).orElseThrow().getStatus()).isEqualTo(TaskStatus.SUCCESS);
    }

    private TaskEntity saveRunningTask() {
        TaskEntity task = new TaskEntity();
        task.setPrompt("service finalization test");
        task.setStatus(TaskStatus.RUNNING);
        task.setRetryCount(0);
        task.setMaxRetry(3);
        task.setTimeoutSeconds(30);
        task.setTimeoutAt(LocalDateTime.now().plusSeconds(30));
        return taskRepository.save(task);
    }

    private LlmResponse successResponse() {
        LlmResponse response = new LlmResponse();
        response.setProvider("mock");
        response.setModel("mock-llm");
        response.setContent("content");
        response.setSuccess(true);
        response.setPromptTokenCount(1);
        response.setCompletionTokenCount(2);
        response.setTotalTokenCount(3);
        response.setLatencyMs(5L);
        return response;
    }
}
