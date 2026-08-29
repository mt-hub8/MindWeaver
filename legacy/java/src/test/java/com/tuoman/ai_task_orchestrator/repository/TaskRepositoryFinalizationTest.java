package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.TaskEntity;
import com.tuoman.ai_task_orchestrator.enums.TaskStatus;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.transaction.annotation.Transactional;

import java.time.LocalDateTime;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest(properties = "spring.task.scheduling.enabled=false")
@Transactional
class TaskRepositoryFinalizationTest {

    private static final String TIMEOUT_ERROR_MESSAGE = "任务执行超时";

    @Autowired
    private TaskRepository taskRepository;

    @Test
    void runningTaskShouldBeMarkedSucceeded() {
        TaskEntity task = saveTask(TaskStatus.RUNNING);

        int updated = taskRepository.markSucceededIfRunning(
                task.getId(),
                TaskStatus.SUCCESS,
                TaskStatus.RUNNING,
                "result",
                "mock-llm",
                "rendered",
                "default_task_prompt"
        );

        assertThat(updated).isEqualTo(1);
        TaskEntity saved = taskRepository.findById(task.getId()).orElseThrow();
        assertThat(saved.getStatus()).isEqualTo(TaskStatus.SUCCESS);
        assertThat(saved.getResultContent()).isEqualTo("result");
    }

    @Test
    void failedTaskShouldNotBeOverwrittenToSuccess() {
        TaskEntity task = saveTask(TaskStatus.FAILED, "execution failed");

        int updated = taskRepository.markSucceededIfRunning(
                task.getId(),
                TaskStatus.SUCCESS,
                TaskStatus.RUNNING,
                "result",
                "mock-llm",
                "rendered",
                "default_task_prompt"
        );

        assertThat(updated).isZero();
        assertThat(taskRepository.findById(task.getId()).orElseThrow().getStatus()).isEqualTo(TaskStatus.FAILED);
    }

    @Test
    void timedOutTaskShouldNotBeOverwrittenToSuccess() {
        TaskEntity task = saveTask(TaskStatus.FAILED, TIMEOUT_ERROR_MESSAGE);

        int updated = taskRepository.markSucceededIfRunning(
                task.getId(),
                TaskStatus.SUCCESS,
                TaskStatus.RUNNING,
                "result",
                "mock-llm",
                "rendered",
                "default_task_prompt"
        );

        assertThat(updated).isZero();
        assertThat(taskRepository.findById(task.getId()).orElseThrow().getErrorMessage()).isEqualTo(TIMEOUT_ERROR_MESSAGE);
    }

    @Test
    void successTaskShouldNotBeOverwrittenToSuccessAgain() {
        TaskEntity task = saveTask(TaskStatus.SUCCESS);

        int updated = taskRepository.markSucceededIfRunning(
                task.getId(),
                TaskStatus.SUCCESS,
                TaskStatus.RUNNING,
                "new-result",
                "mock-llm",
                "rendered",
                "default_task_prompt"
        );

        assertThat(updated).isZero();
    }

    @Test
    void runningTaskShouldBeMarkedFailed() {
        TaskEntity task = saveTask(TaskStatus.RUNNING);

        int updated = taskRepository.markFailedIfRunning(
                task.getId(),
                TaskStatus.FAILED,
                TaskStatus.RUNNING,
                "failed"
        );

        assertThat(updated).isEqualTo(1);
        assertThat(taskRepository.findById(task.getId()).orElseThrow().getStatus()).isEqualTo(TaskStatus.FAILED);
    }

    @Test
    void successTaskShouldNotBeOverwrittenToFailed() {
        TaskEntity task = saveTask(TaskStatus.SUCCESS);

        int updated = taskRepository.markFailedIfRunning(
                task.getId(),
                TaskStatus.FAILED,
                TaskStatus.RUNNING,
                "failed"
        );

        assertThat(updated).isZero();
        assertThat(taskRepository.findById(task.getId()).orElseThrow().getStatus()).isEqualTo(TaskStatus.SUCCESS);
    }

    @Test
    void runningTaskShouldBeMarkedTimedOut() {
        TaskEntity task = saveTask(TaskStatus.RUNNING);

        int updated = taskRepository.markTimedOutIfRunning(
                task.getId(),
                TaskStatus.FAILED,
                TaskStatus.RUNNING,
                TIMEOUT_ERROR_MESSAGE
        );

        assertThat(updated).isEqualTo(1);
        TaskEntity saved = taskRepository.findById(task.getId()).orElseThrow();
        assertThat(saved.getStatus()).isEqualTo(TaskStatus.FAILED);
        assertThat(saved.getErrorMessage()).isEqualTo(TIMEOUT_ERROR_MESSAGE);
    }

    @Test
    void successTaskShouldNotBeOverwrittenToTimedOut() {
        TaskEntity task = saveTask(TaskStatus.SUCCESS);

        int updated = taskRepository.markTimedOutIfRunning(
                task.getId(),
                TaskStatus.FAILED,
                TaskStatus.RUNNING,
                TIMEOUT_ERROR_MESSAGE
        );

        assertThat(updated).isZero();
        assertThat(taskRepository.findById(task.getId()).orElseThrow().getStatus()).isEqualTo(TaskStatus.SUCCESS);
    }

    @Test
    void cancelledTaskShouldNotBeOverwrittenToSuccess() {
        TaskEntity task = saveTask(TaskStatus.CANCELLED);

        int updated = taskRepository.markSucceededIfRunning(
                task.getId(),
                TaskStatus.SUCCESS,
                TaskStatus.RUNNING,
                "result",
                "mock-llm",
                "rendered",
                "default_task_prompt"
        );

        assertThat(updated).isZero();
    }

    @Test
    void cancelledTaskShouldNotBeOverwrittenToFailed() {
        TaskEntity task = saveTask(TaskStatus.CANCELLED);

        int updated = taskRepository.markFailedIfRunning(
                task.getId(),
                TaskStatus.FAILED,
                TaskStatus.RUNNING,
                "failed"
        );

        assertThat(updated).isZero();
    }

    private TaskEntity saveTask(TaskStatus status) {
        return saveTask(status, null);
    }

    private TaskEntity saveTask(TaskStatus status, String errorMessage) {
        TaskEntity task = new TaskEntity();
        task.setPrompt("finalization test");
        task.setStatus(status);
        task.setRetryCount(0);
        task.setMaxRetry(3);
        task.setTimeoutSeconds(30);
        task.setTimeoutAt(LocalDateTime.now().plusSeconds(30));
        task.setErrorMessage(errorMessage);
        return taskRepository.save(task);
    }
}
