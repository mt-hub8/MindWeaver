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
/**
 * V0.6/V1.x Task 后台执行服务。
 *
 * Consumer 进入这里后，先通过 TaskService 原子 claim 任务，再创建 attempt、渲染
 * Prompt Template、经 ModelRouter 选择模型并调用 LlmClient。输出、usage 和 chunks
 * 最终写回 Task/Attempt/OutputChunk。
 *
 * 关键不变量：执行线程写终态前必须重新检查 task 是否仍 RUNNING，避免取消、
 * 超时或重复消费被后完成的线程覆盖。
 */
public class TaskExecutionService {

    private static final String DEFAULT_TEMPLATE_CODE = "default_task_prompt";

    private final TaskService taskService;

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
            // 阶段 1：原子 claim。
            // 重复 MQ 消息、重复 retry 或已取消任务都会在这里自然退出。
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

            // 阶段 2：模拟执行延迟并检查协作式取消。
            // 这保留 V0.6 mock execution 的闭环，也验证 V0.10 cancel/timeout 竞态。
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

            // 阶段 3：读取任务输入，渲染 Prompt Template，并通过 ModelRouter 选择模型。
            // Prompt 渲染失败属于任务失败，不能跳过模板直接拼接 prompt。
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

            // 阶段 4：调用 LlmClient。
            // V1.0 的 LLM abstraction 让执行链路不直接绑定 mock、本地或外部模型厂商。
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

            // 阶段 5：根据 LLM 结果写 SUCCESS / RETRY_PENDING / FAILED。
            // quality 或 usage metadata 只是诊断信息，不应绕过状态机直接改结果。
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

            boolean finalized = taskService.finalizeSuccessfulExecution(
                    taskId,
                    attempt.getId(),
                    response,
                    renderedPrompt,
                    DEFAULT_TEMPLATE_CODE
            );

            if (!finalized) {
                taskAttemptService.markFailed(
                        attempt.getId(),
                        "Task success finalization rejected",
                        response,
                        renderedPrompt,
                        DEFAULT_TEMPLATE_CODE
                );
                log.info("Task success finalization rejected, taskId={}", taskId);
                return;
            }

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
        // 失败处理先尊重 cancel/timeout 等外部终态，再决定是否进入 retry。
        // 这样异常路径不会把用户取消的任务重新排入队列。
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

        if (taskService.tryMarkTaskRetryPending(taskId, errorMessage)) {
            log.info(
                    "task_final_status_updated taskId={} attemptId={} attemptNo={} status={}",
                    taskId,
                    attempt == null ? null : attempt.getId(),
                    attempt == null ? null : attempt.getAttemptNo(),
                    "RETRY_PENDING"
            );
            log.info("Task entered retry pending, taskId={}", taskId);
            return;
        }

        if (taskService.tryMarkTaskFailed(taskId, errorMessage)) {
            log.info(
                    "task_final_status_updated taskId={} attemptId={} attemptNo={} status={}",
                    taskId,
                    attempt == null ? null : attempt.getId(),
                    attempt == null ? null : attempt.getAttemptNo(),
                    "FAILED"
            );
            return;
        }

        log.info("Task failure finalization rejected, taskId={}", taskId);
    }
}
