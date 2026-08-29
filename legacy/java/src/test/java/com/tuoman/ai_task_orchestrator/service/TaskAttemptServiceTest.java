package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.TaskAttemptResponse;
import com.tuoman.ai_task_orchestrator.entity.TaskAttemptEntity;
import com.tuoman.ai_task_orchestrator.enums.TaskAttemptStatus;
import com.tuoman.ai_task_orchestrator.entity.TaskEntity;
import com.tuoman.ai_task_orchestrator.enums.TaskStatus;
import com.tuoman.ai_task_orchestrator.llm.LlmResponse;
import com.tuoman.ai_task_orchestrator.repository.TaskAttemptRepository;
import com.tuoman.ai_task_orchestrator.repository.TaskRepository;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.dao.DataIntegrityViolationException;
import org.springframework.http.HttpStatus;
import org.springframework.transaction.annotation.Transactional;
import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.common.error.ErrorCode;

import java.util.Arrays;
import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

@SpringBootTest(properties = "spring.task.scheduling.enabled=false")
@Transactional
class TaskAttemptServiceTest {

    @Autowired
    private TaskAttemptService taskAttemptService;

    @Autowired
    private TaskAttemptRepository taskAttemptRepository;

    @Autowired
    private TaskRepository taskRepository;

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

    @Test
    void getAttemptsShouldReturnResponsesOrderedByAttemptNo() {
        TaskEntity task = saveTask("attempt history task");
        TaskAttemptEntity second = newRunningAttempt(task.getId(), 2);
        second.setLlmProvider("mock");
        second.setLlmModel("mock-smart");
        second.setPromptTemplateCode("default_task_prompt");
        second.setPromptTokenCount(10);
        second.setCompletionTokenCount(20);
        second.setTotalTokenCount(30);
        second.setLlmLatencyMs(40L);
        second.setErrorMessage("retry failed");

        TaskAttemptEntity first = newRunningAttempt(task.getId(), 1);
        first.setLlmProvider("mock");
        first.setLlmModel("mock-fast");
        first.setPromptTemplateCode("default_task_prompt");
        first.setPromptTokenCount(1);
        first.setCompletionTokenCount(2);
        first.setTotalTokenCount(3);
        first.setLlmLatencyMs(4L);

        taskAttemptRepository.saveAndFlush(second);
        taskAttemptRepository.saveAndFlush(first);

        List<TaskAttemptResponse> responses = taskAttemptService.getAttempts(task.getId());

        assertThat(responses).hasSize(2);
        assertThat(responses).extracting(TaskAttemptResponse::getAttemptNo).containsExactly(1, 2);
        assertThat(responses.get(0).getTaskId()).isEqualTo(task.getId());
        assertThat(responses.get(0).getStatus()).isEqualTo(TaskAttemptStatus.RUNNING);
        assertThat(responses.get(0).getWorkerId()).isEqualTo("test-worker");
        assertThat(responses.get(0).getLlmProvider()).isEqualTo("mock");
        assertThat(responses.get(0).getLlmModel()).isEqualTo("mock-fast");
        assertThat(responses.get(0).getPromptTemplateCode()).isEqualTo("default_task_prompt");
        assertThat(responses.get(0).getPromptTokenCount()).isEqualTo(1);
        assertThat(responses.get(0).getCompletionTokenCount()).isEqualTo(2);
        assertThat(responses.get(0).getTotalTokenCount()).isEqualTo(3);
        assertThat(responses.get(0).getLlmLatencyMs()).isEqualTo(4L);
        assertThat(responses.get(1).getErrorMessage()).isEqualTo("retry failed");
    }

    @Test
    void getAttemptsShouldReturnEmptyListWhenTaskHasNoAttempts() {
        TaskEntity task = saveTask("task without attempts");

        assertThat(taskAttemptService.getAttempts(task.getId())).isEmpty();
    }

    @Test
    void getAttemptsShouldReturnNotFoundWhenTaskDoesNotExist() {
        assertThatThrownBy(() -> taskAttemptService.getAttempts(999999L))
                .isInstanceOf(BusinessException.class)
                .satisfies(exception -> {
                    BusinessException businessException = (BusinessException) exception;
                    assertThat(businessException.getErrorCode()).isEqualTo(ErrorCode.TASK_NOT_FOUND);
                    assertThat(businessException.getHttpStatus()).isEqualTo(HttpStatus.NOT_FOUND);
                });
    }

    @Test
    void taskAttemptResponseShouldNotExposeRenderedPrompt() {
        assertThat(Arrays.stream(TaskAttemptResponse.class.getDeclaredFields())
                .map(field -> field.getName()))
                .doesNotContain("renderedPrompt");
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

    private TaskEntity saveTask(String prompt) {
        TaskEntity task = new TaskEntity();
        task.setPrompt(prompt);
        task.setStatus(TaskStatus.PENDING);
        task.setRetryCount(0);
        task.setMaxRetry(3);
        task.setTimeoutSeconds(30);
        return taskRepository.saveAndFlush(task);
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
