package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.entity.TaskAttemptEntity;
import com.tuoman.ai_task_orchestrator.enums.TaskAttemptStatus;
import com.tuoman.ai_task_orchestrator.llm.LlmResponse;
import com.tuoman.ai_task_orchestrator.repository.TaskAttemptRepository;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.dao.DataIntegrityViolationException;
import org.springframework.transaction.annotation.Transactional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

@SpringBootTest(properties = "spring.task.scheduling.enabled=false")
@Transactional
class TaskAttemptServiceTest {

    @Autowired
    private TaskAttemptService taskAttemptService;

    @Autowired
    private TaskAttemptRepository taskAttemptRepository;

    @Test
    void createRunningAttemptShouldIncreaseAttemptNo() {
        TaskAttemptEntity first = taskAttemptService.createRunningAttempt(1001L);
        TaskAttemptEntity second = taskAttemptService.createRunningAttempt(1001L);

        assertThat(first.getAttemptNo()).isEqualTo(1);
        assertThat(second.getAttemptNo()).isEqualTo(2);
        assertThat(first.getStatus()).isEqualTo(TaskAttemptStatus.RUNNING);
        assertThat(second.getStatus()).isEqualTo(TaskAttemptStatus.RUNNING);
    }

    @Test
    void sameTaskAttemptNoShouldBeUnique() {
        TaskAttemptEntity first = newRunningAttempt(1002L, 1);
        TaskAttemptEntity duplicate = newRunningAttempt(1002L, 1);

        taskAttemptRepository.saveAndFlush(first);

        assertThatThrownBy(() -> taskAttemptRepository.saveAndFlush(duplicate))
                .isInstanceOf(DataIntegrityViolationException.class);
    }

    @Test
    void runningAttemptCanBeMarkedSuccessWithLlmMetadata() {
        TaskAttemptEntity attempt = taskAttemptService.createRunningAttempt(1003L);
        LlmResponse response = successResponse();

        TaskAttemptEntity saved = taskAttemptService.markSuccess(
                attempt.getId(),
                response,
                "rendered prompt",
                "default_task_prompt"
        );

        assertThat(saved.getStatus()).isEqualTo(TaskAttemptStatus.SUCCESS);
        assertThat(saved.getLlmProvider()).isEqualTo("mock");
        assertThat(saved.getLlmModel()).isEqualTo("mock-llm");
        assertThat(saved.getPromptTokenCount()).isEqualTo(3);
        assertThat(saved.getCompletionTokenCount()).isEqualTo(4);
        assertThat(saved.getTotalTokenCount()).isEqualTo(7);
        assertThat(saved.getLlmLatencyMs()).isEqualTo(12L);
        assertThat(saved.getPromptTemplateCode()).isEqualTo("default_task_prompt");
        assertThat(saved.getRenderedPrompt()).isEqualTo("rendered prompt");
        assertThat(saved.getFinishedAt()).isNotNull();
    }

    @Test
    void runningAttemptCanBeMarkedFailedWithErrorMessage() {
        TaskAttemptEntity attempt = taskAttemptService.createRunningAttempt(1004L);
        LlmResponse response = failedResponse();

        TaskAttemptEntity saved = taskAttemptService.markFailed(
                attempt.getId(),
                "Mock LLM execution failed",
                response,
                "rendered prompt",
                "default_task_prompt"
        );

        assertThat(saved.getStatus()).isEqualTo(TaskAttemptStatus.FAILED);
        assertThat(saved.getErrorMessage()).isEqualTo("Mock LLM execution failed");
        assertThat(saved.getLlmProvider()).isEqualTo("mock");
        assertThat(saved.getLlmModel()).isEqualTo("mock-llm");
        assertThat(saved.getTotalTokenCount()).isEqualTo(3);
        assertThat(saved.getFinishedAt()).isNotNull();
    }

    private TaskAttemptEntity newRunningAttempt(Long taskId, Integer attemptNo) {
        TaskAttemptEntity attempt = new TaskAttemptEntity();
        attempt.setTaskId(taskId);
        attempt.setAttemptNo(attemptNo);
        attempt.setStatus(TaskAttemptStatus.RUNNING);
        attempt.setWorkerId("test-worker");
        attempt.setStartedAt(java.time.LocalDateTime.now());
        return attempt;
    }

    private LlmResponse successResponse() {
        LlmResponse response = new LlmResponse();
        response.setProvider("mock");
        response.setModel("mock-llm");
        response.setSuccess(true);
        response.setPromptTokenCount(3);
        response.setCompletionTokenCount(4);
        response.setTotalTokenCount(7);
        response.setLatencyMs(12L);
        return response;
    }

    private LlmResponse failedResponse() {
        LlmResponse response = successResponse();
        response.setSuccess(false);
        response.setCompletionTokenCount(0);
        response.setTotalTokenCount(3);
        response.setErrorMessage("Mock LLM execution failed");
        return response;
    }
}
