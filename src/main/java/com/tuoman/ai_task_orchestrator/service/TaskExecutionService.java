package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.enums.TaskStatus;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;

@Slf4j
@Service
@RequiredArgsConstructor
public class TaskExecutionService {

    private final TaskService taskService;

    public void executeTask(Long taskId) {
        log.info("Start executing task, taskId={}", taskId);

        try {
            boolean started = taskService.tryStartTaskExecution(taskId, "任务开始执行");
            if (!started) {
                log.info("Ignore duplicated task execution message, taskId={}", taskId);
                return;
            }

            simulateTaskExecution(taskId);

            taskService.updateTaskStatus(
                    taskId,
                    TaskStatus.SUCCESS,
                    "任务执行成功"
            );

            log.info("Finish executing task, taskId={}", taskId);
        } catch (Exception e) {
            log.error("Task execution failed, taskId={}", taskId, e);
            try {
                taskService.markTaskRetryPending(taskId, e.getMessage());
                log.info("Task entered retry pending, taskId={}", taskId);
            } catch (Exception retryPendingException) {
                log.error("Task cannot retry, mark as failed, taskId={}", taskId, retryPendingException);
                taskService.markTaskFailed(taskId, e.getMessage());
            }
        }
    }

    private void simulateTaskExecution(Long taskId) {
        try {
            log.info("Simulating task execution, taskId={}", taskId);
            String prompt = taskService.getTaskById(taskId).getPrompt();
            if (prompt != null && (prompt.contains("fail") || prompt.contains("失败"))) {
                throw new RuntimeException("模拟任务执行失败");
            }
            Thread.sleep(3000);
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            throw new RuntimeException("任务执行被中断", e);
        }
    }
}
