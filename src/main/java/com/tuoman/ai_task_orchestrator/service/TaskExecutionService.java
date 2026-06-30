package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.enums.TaskStatus;
import com.tuoman.ai_task_orchestrator.llm.LlmClient;
import com.tuoman.ai_task_orchestrator.llm.LlmRequest;
import com.tuoman.ai_task_orchestrator.llm.LlmResponse;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;

@Slf4j
@Service
@RequiredArgsConstructor
public class TaskExecutionService {

    private final TaskService taskService;

    private final LlmClient llmClient;

    public void executeTask(Long taskId) {
        log.info("Start executing task, taskId={}", taskId);

        try {
            boolean started = taskService.tryStartTaskExecution(taskId, "任务开始执行");
            if (!started) {
                log.info("Ignore duplicated task execution message, taskId={}", taskId);
                return;
            }

            boolean completed = simulateExecutionDelay(taskId);
            if (!completed) {
                return;
            }

            if (!taskService.isTaskRunning(taskId)) {
                log.info("Task is no longer running, skip LLM execution, taskId={}", taskId);
                return;
            }

            String prompt = taskService.getTaskById(taskId).getPrompt();
            LlmRequest request = new LlmRequest();
            request.setTaskId(taskId);
            request.setPrompt(prompt);
            request.setModel("mock-llm");

            log.info("Calling LlmClient, taskId={}, model={}", taskId, request.getModel());
            LlmResponse response = llmClient.generate(request);

            if (!response.isSuccess()) {
                log.warn("LLM execution returned failure, taskId={}, errorMessage={}",
                        taskId,
                        response.getErrorMessage());
                handleTaskExecutionFailure(taskId, response.getErrorMessage());
                return;
            }

            log.info("LLM execution succeeded, taskId={}, model={}", taskId, response.getModel());

            if (taskService.isTaskCancelled(taskId)) {
                log.info("Task execution cancelled, taskId={}", taskId);
                return;
            }

            if (!taskService.isTaskRunning(taskId)) {
                log.info("Task is no longer running, skip marking success, taskId={}", taskId);
                return;
            }

            taskService.updateTaskStatus(
                    taskId,
                    TaskStatus.SUCCESS,
                    "LLM 任务执行成功"
            );

            log.info("Finish executing task, taskId={}", taskId);
        } catch (Exception e) {
            log.error("LLM execution exception, taskId={}", taskId, e);
            handleTaskExecutionFailure(taskId, e.getMessage());
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
            throw new RuntimeException("任务执行被中断", e);
        }
    }

    private void handleTaskExecutionFailure(Long taskId, String errorMessage) {
        if (taskService.isTaskCancelled(taskId)) {
            log.info("Task execution cancelled, taskId={}", taskId);
            return;
        }

        if (!taskService.isTaskRunning(taskId)) {
            log.info("Task is no longer running, skip failure handling, taskId={}", taskId);
            return;
        }

        try {
            taskService.markTaskRetryPending(taskId, errorMessage);
            log.info("Task entered retry pending, taskId={}", taskId);
        } catch (Exception retryPendingException) {
            log.error("Task cannot retry, mark as failed, taskId={}", taskId, retryPendingException);
            taskService.markTaskFailed(taskId, errorMessage);
        }
    }
}
