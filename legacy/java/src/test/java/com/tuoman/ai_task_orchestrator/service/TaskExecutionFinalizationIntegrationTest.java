package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.entity.TaskEntity;
import com.tuoman.ai_task_orchestrator.enums.TaskAttemptStatus;
import com.tuoman.ai_task_orchestrator.enums.TaskStatus;
import com.tuoman.ai_task_orchestrator.llm.LlmResponse;
import com.tuoman.ai_task_orchestrator.repository.TaskAttemptRepository;
import com.tuoman.ai_task_orchestrator.repository.TaskOutputChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.TaskRepository;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.transaction.annotation.Transactional;

import java.time.LocalDateTime;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest(properties = "spring.task.scheduling.enabled=false")
@Transactional
class TaskExecutionFinalizationIntegrationTest {

    @Autowired
    private TaskService taskService;

    @Autowired
    private TaskAttemptService taskAttemptService;

    @Autowired
    private TaskRepository taskRepository;

    @Autowired
    private TaskAttemptRepository taskAttemptRepository;

    @Autowired
    private TaskOutputChunkRepository taskOutputChunkRepository;

    @Test
    void successfulFinalizationShouldPersistTaskAttemptAndOutputChunks() {
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
        assertThat(taskRepository.findById(task.getId()).orElseThrow().getStatus()).isEqualTo(TaskStatus.SUCCESS);
        assertThat(taskAttemptRepository.findById(attempt.getId()).orElseThrow().getStatus())
                .isEqualTo(TaskAttemptStatus.SUCCESS);
        assertThat(taskOutputChunkRepository.findByTaskIdOrderByChunkIndexAsc(task.getId())).isNotEmpty();
    }

    @Test
    void rejectedFinalizationShouldNotPersistSuccessAttemptOrOutputChunks() {
        TaskEntity task = saveRunningTask();
        var attempt = taskAttemptService.createRunningAttempt(task.getId());

        task.setStatus(TaskStatus.FAILED);
        task.setErrorMessage("任务执行超时");
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
        assertThat(taskAttemptRepository.findById(attempt.getId()).orElseThrow().getStatus())
                .isEqualTo(TaskAttemptStatus.RUNNING);
        assertThat(taskOutputChunkRepository.findByTaskIdOrderByChunkIndexAsc(task.getId())).isEmpty();
    }

    private TaskEntity saveRunningTask() {
        TaskEntity task = new TaskEntity();
        task.setPrompt("execution finalization test");
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
        response.setContent("abcdefghijklmnopqrstuvwxyz1234567890");
        response.setSuccess(true);
        response.setPromptTokenCount(1);
        response.setCompletionTokenCount(2);
        response.setTotalTokenCount(3);
        response.setLatencyMs(5L);
        return response;
    }
}
