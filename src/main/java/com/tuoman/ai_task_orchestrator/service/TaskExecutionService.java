package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.TaskDetailResponse;
import com.tuoman.ai_task_orchestrator.entity.PromptTemplateEntity;
import com.tuoman.ai_task_orchestrator.entity.TaskAttemptEntity;
import com.tuoman.ai_task_orchestrator.llm.LlmClient;
import com.tuoman.ai_task_orchestrator.llm.LlmRequest;
import com.tuoman.ai_task_orchestrator.llm.LlmResponse;
import com.tuoman.ai_task_orchestrator.llm.ModelRouter;
import com.tuoman.ai_task_orchestrator.prompt.PromptTemplateRenderer;
import com.tuoman.ai_task_orchestrator.repository.PromptTemplateRepository;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;

import java.util.Map;

@Slf4j
@Service
@RequiredArgsConstructor
public class TaskExecutionService {

    private static final String DEFAULT_TEMPLATE_CODE = "default_task_prompt";

    private final TaskService taskService;

    private final TaskOutputChunkService taskOutputChunkService;

    private final TaskAttemptService taskAttemptService;

    private final LlmClient llmClient;

    private final ModelRouter modelRouter;

    private final PromptTemplateRepository promptTemplateRepository;

    private final PromptTemplateRenderer promptTemplateRenderer;

    public void executeTask(Long taskId) {
        log.info("Start executing task, taskId={}", taskId);

        TaskAttemptEntity attempt = null;
        String renderedPrompt = null;
        LlmResponse response = null;

        try {
            boolean started = taskService.tryStartTaskExecution(taskId, "Task execution started");
            if (!started) {
                log.info("task_execution_claim_conflict taskId={}", taskId);
                log.info("Ignore duplicated task execution message, taskId={}", taskId);
                return;
            }

            attempt = taskAttemptService.createRunningAttempt(taskId);
            log.info(
                    "task_execution_claimed taskId={} attemptId={} attemptNo={} status={}",
                    taskId,
                    attempt.getId(),
                    attempt.getAttemptNo(),
                    attempt.getStatus()
            );

            boolean completed = simulateExecutionDelay(taskId);
            if (!completed) {
                taskAttemptService.markCancelled(attempt.getId(), "Task execution cancelled");
                return;
            }

            if (!taskService.isTaskRunning(taskId)) {
                taskAttemptService.markFailed(attempt.getId(), "Task is no longer running", response, renderedPrompt, DEFAULT_TEMPLATE_CODE);
                log.info("Task is no longer running, skip LLM execution, taskId={}", taskId);
                return;
            }

            TaskDetailResponse task = taskService.getTaskById(taskId);
            String prompt = task.getPrompt();
            String selectedModel = modelRouter.route(task.getRequestedModel());
            log.info("Find prompt template, taskId={}, templateCode={}", taskId, DEFAULT_TEMPLATE_CODE);
            PromptTemplateEntity template = promptTemplateRepository
                    .findByTemplateCodeAndEnabledTrue(DEFAULT_TEMPLATE_CODE)
                    .orElseThrow(() -> new IllegalStateException(
                            "Default prompt template not found: " + DEFAULT_TEMPLATE_CODE
                    ));

            Map<String, String> variables = Map.of(
                    "prompt", prompt == null ? "" : prompt,
                    "taskId", String.valueOf(taskId),
                    "model", selectedModel
            );
            renderedPrompt = promptTemplateRenderer.render(template.getTemplateContent(), variables);
            log.info("Prompt template rendered, taskId={}, templateCode={}, renderedPromptLength={}",
                    taskId,
                    DEFAULT_TEMPLATE_CODE,
                    renderedPrompt.length());

            LlmRequest request = new LlmRequest();
            request.setTaskId(taskId);
            request.setPrompt(renderedPrompt);
            request.setModel(selectedModel);

            log.info("Calling LlmClient, taskId={}, attemptId={}, attemptNo={}, model={}",
                    taskId,
                    attempt.getId(),
                    attempt.getAttemptNo(),
                    request.getModel());
            response = llmClient.generate(request);
            if (response != null) {
                taskService.saveLlmMetadata(
                        taskId,
                        response.getProvider(),
                        response.getModel(),
                        response.getPromptTokenCount(),
                        response.getCompletionTokenCount(),
                        response.getTotalTokenCount(),
                        response.getLatencyMs()
                );
                log.info(
                        "llm_execution_completed taskId={} attemptId={} attemptNo={} status={} provider={} model={} latencyMs={} totalTokenCount={}",
                        taskId,
                        attempt.getId(),
                        attempt.getAttemptNo(),
                        response.isSuccess() ? "SUCCESS" : "FAILED",
                        response.getProvider(),
                        response.getModel(),
                        response.getLatencyMs(),
                        response.getTotalTokenCount()
                );
            }

            if (response == null || !response.isSuccess()) {
                String errorMessage = response == null ? "LLM response is null" : response.getErrorMessage();
                taskAttemptService.markFailed(attempt.getId(), errorMessage, response, renderedPrompt, DEFAULT_TEMPLATE_CODE);
                log.warn("LLM execution returned failure, taskId={}, errorMessage={}", taskId, errorMessage);
                handleTaskExecutionFailure(taskId, attempt, errorMessage);
                return;
            }

            log.info("LLM execution succeeded, taskId={}, model={}", taskId, response.getModel());

            if (taskService.isTaskCancelled(taskId)) {
                taskAttemptService.markCancelled(attempt.getId(), "Task execution cancelled");
                log.info("Task execution cancelled, taskId={}", taskId);
                return;
            }

            if (!taskService.isTaskRunning(taskId)) {
                taskAttemptService.markFailed(attempt.getId(), "Task is no longer running", response, renderedPrompt, DEFAULT_TEMPLATE_CODE);
                log.info("Task is no longer running, skip marking success, taskId={}", taskId);
                return;
            }

            taskOutputChunkService.saveChunks(taskId, response.getContent());

            if (!taskService.isTaskRunning(taskId)) {
                taskAttemptService.markFailed(attempt.getId(), "Task is no longer running after saving chunks", response, renderedPrompt, DEFAULT_TEMPLATE_CODE);
                log.info("Task is no longer running, skip marking success after saving chunks, taskId={}", taskId);
                return;
            }

            taskAttemptService.markSuccess(attempt.getId(), response, renderedPrompt, DEFAULT_TEMPLATE_CODE);

            taskService.markTaskSucceeded(
                    taskId,
                    response.getContent(),
                    response.getModel(),
                    renderedPrompt,
                    DEFAULT_TEMPLATE_CODE
            );

            log.info(
                    "task_final_status_updated taskId={} attemptId={} attemptNo={} status={}",
                    taskId,
                    attempt.getId(),
                    attempt.getAttemptNo(),
                    "SUCCESS"
            );
            log.info("Finish executing task, taskId={}", taskId);
        } catch (Exception e) {
            log.error("LLM execution exception, taskId={}", taskId, e);
            if (attempt != null) {
                taskAttemptService.markFailed(attempt.getId(), e.getMessage(), response, renderedPrompt, DEFAULT_TEMPLATE_CODE);
            }
            handleTaskExecutionFailure(taskId, attempt, e.getMessage());
        }
    }

    private boolean simulateExecutionDelay(Long taskId) {
        try {
            log.info("Simulating task execution, taskId={}", taskId);

            for (int i = 0; i < 5; i++) {
                Thread.sleep(1000);
                if (taskService.isTaskCancelled(taskId)) {
                    log.info("Task execution cancelled, taskId={}", taskId);
                    return false;
                }
            }

            return true;
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            throw new RuntimeException("Task execution interrupted", e);
        }
    }

    private void handleTaskExecutionFailure(Long taskId, TaskAttemptEntity attempt, String errorMessage) {
        if (taskService.isTaskCancelled(taskId)) {
            if (attempt != null) {
                taskAttemptService.markCancelled(attempt.getId(), "Task execution cancelled");
            }
            log.info("Task execution cancelled, taskId={}", taskId);
            return;
        }

        if (!taskService.isTaskRunning(taskId)) {
            log.info("Task is no longer running, skip failure handling, taskId={}", taskId);
            return;
        }

        try {
            taskService.markTaskRetryPending(taskId, errorMessage);
            log.info(
                    "task_final_status_updated taskId={} attemptId={} attemptNo={} status={}",
                    taskId,
                    attempt == null ? null : attempt.getId(),
                    attempt == null ? null : attempt.getAttemptNo(),
                    "RETRY_PENDING"
            );
            log.info("Task entered retry pending, taskId={}", taskId);
        } catch (Exception retryPendingException) {
            log.error("Task cannot retry, mark as failed, taskId={}", taskId, retryPendingException);
            taskService.markTaskFailed(taskId, errorMessage);
            log.info(
                    "task_final_status_updated taskId={} attemptId={} attemptNo={} status={}",
                    taskId,
                    attempt == null ? null : attempt.getId(),
                    attempt == null ? null : attempt.getAttemptNo(),
                    "FAILED"
            );
        }
    }
}
