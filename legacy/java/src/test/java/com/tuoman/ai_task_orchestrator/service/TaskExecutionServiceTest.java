package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.TaskDetailResponse;
import com.tuoman.ai_task_orchestrator.entity.PromptTemplateEntity;
import com.tuoman.ai_task_orchestrator.entity.TaskAttemptEntity;
import com.tuoman.ai_task_orchestrator.enums.TaskAttemptStatus;
import com.tuoman.ai_task_orchestrator.enums.TaskStatus;
import com.tuoman.ai_task_orchestrator.llm.LlmClient;
import com.tuoman.ai_task_orchestrator.llm.LlmRequest;
import com.tuoman.ai_task_orchestrator.llm.LlmResponse;
import com.tuoman.ai_task_orchestrator.llm.ModelRouter;
import com.tuoman.ai_task_orchestrator.prompt.PromptTemplateRenderer;
import com.tuoman.ai_task_orchestrator.repository.PromptTemplateRepository;
import org.junit.jupiter.api.Test;

import java.time.LocalDateTime;
import java.util.Optional;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class TaskExecutionServiceTest {

    private final TaskService taskService = mock(TaskService.class);

    private final TaskAttemptService taskAttemptService = mock(TaskAttemptService.class);

    private final LlmClient llmClient = mock(LlmClient.class);

    private final ModelRouter modelRouter = mock(ModelRouter.class);

    private final PromptTemplateRepository promptTemplateRepository = mock(PromptTemplateRepository.class);

    private final PromptTemplateRenderer promptTemplateRenderer = mock(PromptTemplateRenderer.class);

    private final TaskExecutionService taskExecutionService = new TaskExecutionService(
            taskService,
            taskAttemptService,
            llmClient,
            modelRouter,
            promptTemplateRepository,
            promptTemplateRenderer
    );

    @Test
    void claimFailureShouldNotCreateAttempt() {
        when(taskService.tryStartTaskExecution(1L, "Task execution started")).thenReturn(false);

        taskExecutionService.executeTask(1L);

        verify(taskAttemptService, never()).createRunningAttempt(1L);
        verify(llmClient, never()).generate(any());
    }

    @Test
    void successfulExecutionShouldCreateAndMarkSuccessAttempt() {
        TaskAttemptEntity attempt = runningAttempt();
        LlmResponse response = successResponse();

        when(taskService.tryStartTaskExecution(1L, "Task execution started")).thenReturn(true);
        when(taskAttemptService.createRunningAttempt(1L)).thenReturn(attempt);
        when(taskService.isTaskCancelled(1L)).thenReturn(false);
        when(taskService.isTaskRunning(1L)).thenReturn(true);
        when(taskService.getTaskById(1L)).thenReturn(taskDetail("normal task", "mock-fast"));
        when(modelRouter.route("mock-fast")).thenReturn("mock-fast");
        when(promptTemplateRepository.findByTemplateCodeAndEnabledTrue("default_task_prompt"))
                .thenReturn(Optional.of(template()));
        when(promptTemplateRenderer.render("template {{prompt}}", java.util.Map.of(
                "prompt", "normal task",
                "taskId", "1",
                "model", "mock-fast"
        ))).thenReturn("rendered prompt");
        when(llmClient.generate(any(LlmRequest.class))).thenReturn(response);

        taskExecutionService.executeTask(1L);

        verify(taskAttemptService).createRunningAttempt(1L);
        verify(taskService).finalizeSuccessfulExecution(1L, 10L, response, "rendered prompt", "default_task_prompt");
        verify(taskService).saveLlmMetadata(1L, "mock", "mock-fast", 3, 4, 7, 12L);
    }

    @Test
    void rejectedSuccessFinalizationShouldMarkFailedAttemptAndSkipChunks() {
        TaskAttemptEntity attempt = runningAttempt();
        LlmResponse response = successResponse();

        when(taskService.tryStartTaskExecution(1L, "Task execution started")).thenReturn(true);
        when(taskAttemptService.createRunningAttempt(1L)).thenReturn(attempt);
        when(taskService.isTaskCancelled(1L)).thenReturn(false);
        when(taskService.isTaskRunning(1L)).thenReturn(true);
        when(taskService.getTaskById(1L)).thenReturn(taskDetail("normal task", "mock-fast"));
        when(modelRouter.route("mock-fast")).thenReturn("mock-fast");
        when(promptTemplateRepository.findByTemplateCodeAndEnabledTrue("default_task_prompt"))
                .thenReturn(Optional.of(template()));
        when(promptTemplateRenderer.render(any(), any())).thenReturn("rendered prompt");
        when(llmClient.generate(any(LlmRequest.class))).thenReturn(response);
        when(taskService.finalizeSuccessfulExecution(1L, 10L, response, "rendered prompt", "default_task_prompt"))
                .thenReturn(false);

        taskExecutionService.executeTask(1L);

        verify(taskAttemptService).markFailed(
                10L,
                "Task success finalization rejected",
                response,
                "rendered prompt",
                "default_task_prompt"
        );
        verify(taskAttemptService, never()).markSuccess(any(), any(), any(), any());
    }

    @Test
    void failedLlmExecutionShouldMarkFailedAttemptAndKeepTaskFailureFlow() {
        TaskAttemptEntity attempt = runningAttempt();
        LlmResponse response = failedResponse();

        when(taskService.tryStartTaskExecution(1L, "Task execution started")).thenReturn(true);
        when(taskAttemptService.createRunningAttempt(1L)).thenReturn(attempt);
        when(taskService.isTaskCancelled(1L)).thenReturn(false);
        when(taskService.isTaskRunning(1L)).thenReturn(true);
        when(taskService.getTaskById(1L)).thenReturn(taskDetail("please fail this task", "mock-fast"));
        when(modelRouter.route("mock-fast")).thenReturn("mock-fast");
        when(promptTemplateRepository.findByTemplateCodeAndEnabledTrue("default_task_prompt"))
                .thenReturn(Optional.of(template()));
        when(promptTemplateRenderer.render(any(), any())).thenReturn("rendered prompt");
        when(llmClient.generate(any(LlmRequest.class))).thenReturn(response);

        taskExecutionService.executeTask(1L);

        verify(taskAttemptService).markFailed(
                10L,
                "Mock LLM execution failed",
                response,
                "rendered prompt",
                "default_task_prompt"
        );
        verify(taskService).tryMarkTaskRetryPending(1L, "Mock LLM execution failed");
    }

    private TaskAttemptEntity runningAttempt() {
        TaskAttemptEntity attempt = new TaskAttemptEntity();
        attempt.setId(10L);
        attempt.setTaskId(1L);
        attempt.setAttemptNo(1);
        attempt.setStatus(TaskAttemptStatus.RUNNING);
        return attempt;
    }

    private PromptTemplateEntity template() {
        PromptTemplateEntity template = new PromptTemplateEntity();
        template.setTemplateCode("default_task_prompt");
        template.setTemplateContent("template {{prompt}}");
        return template;
    }

    private TaskDetailResponse taskDetail(String prompt, String requestedModel) {
        return new TaskDetailResponse(
                1L,
                prompt,
                requestedModel,
                TaskStatus.RUNNING,
                null,
                0,
                3,
                null,
                30,
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                LocalDateTime.now(),
                LocalDateTime.now()
        );
    }

    private LlmResponse successResponse() {
        LlmResponse response = new LlmResponse();
        response.setProvider("mock");
        response.setModel("mock-fast");
        response.setContent("content");
        response.setSuccess(true);
        response.setPromptTokenCount(3);
        response.setCompletionTokenCount(4);
        response.setTotalTokenCount(7);
        response.setLatencyMs(12L);
        return response;
    }

    private LlmResponse failedResponse() {
        LlmResponse response = successResponse();
        response.setContent(null);
        response.setSuccess(false);
        response.setCompletionTokenCount(0);
        response.setTotalTokenCount(3);
        response.setErrorMessage("Mock LLM execution failed");
        return response;
    }
}
