package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.mq.TaskDispatchProducer;
import com.tuoman.ai_task_orchestrator.dto.CreateTaskRequest;
import com.tuoman.ai_task_orchestrator.dto.CreateTaskResponse;
import com.tuoman.ai_task_orchestrator.dto.TaskDetailResponse;
import com.tuoman.ai_task_orchestrator.entity.TaskEntity;
import com.tuoman.ai_task_orchestrator.entity.TaskEventEntity;
import com.tuoman.ai_task_orchestrator.enums.TaskEventType;
import com.tuoman.ai_task_orchestrator.enums.TaskStatus;
import com.tuoman.ai_task_orchestrator.repository.TaskEventRepository;
import com.tuoman.ai_task_orchestrator.repository.TaskRepository;
import com.tuoman.ai_task_orchestrator.state.TaskStateMachine;
import lombok.RequiredArgsConstructor;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;
import org.springframework.web.server.ResponseStatusException;

import java.time.LocalDateTime;
import java.util.List;

@Service
@RequiredArgsConstructor
public class TaskService {

    private static final String DEFAULT_ERROR_MESSAGE = "未知错误";

    private static final int MAX_ERROR_MESSAGE_LENGTH = 2000;

    private final TaskRepository taskRepository;

    private final TaskEventRepository taskEventRepository;

    private final TaskStateMachine taskStateMachine;

    @Transactional
    public CreateTaskResponse createTask(CreateTaskRequest request) {
        TaskEntity task = new TaskEntity();
        task.setPrompt(request.getPrompt());
        task.setRequestedModel(request.getModel());
        task.setStatus(TaskStatus.PENDING);
        task.setRetryCount(0);
        task.setMaxRetry(3);
        task.setNextRetryAt(null);
        task.setTimeoutSeconds(30);
        task.setTimeoutAt(null);

        TaskEntity savedTask = taskRepository.save(task);

        recordTaskEvent(
                savedTask.getId(),
                TaskEventType.TASK_CREATED,
                null,
                TaskStatus.PENDING,
                "任务创建成功"
        );

        taskDispatchProducer.sendTaskCreatedMessage(savedTask.getId());

        return new CreateTaskResponse(savedTask.getId(), savedTask.getStatus());
    }

    @Transactional(readOnly = true)
    public TaskDetailResponse getTaskById(Long taskId) {
        TaskEntity task = findTaskOrThrow(taskId);
        return toTaskDetailResponse(task);
    }

    @Transactional
    public TaskDetailResponse updateTaskStatus(Long taskId, TaskStatus targetStatus, String message) {
        TaskEntity task = findTaskOrThrow(taskId);

        TaskStatus currentStatus = task.getStatus();

        if (!taskStateMachine.canTransit(currentStatus, targetStatus)) {
            throw new ResponseStatusException(
                    HttpStatus.BAD_REQUEST,
                    "非法状态流转：" + currentStatus + " -> " + targetStatus
            );
        }

        task.setStatus(targetStatus);

        TaskEntity savedTask = taskRepository.save(task);

        recordTaskEvent(
                savedTask.getId(),
                TaskEventType.STATUS_CHANGED,
                currentStatus,
                targetStatus,
                message
        );

        return toTaskDetailResponse(savedTask);
    }

    @Transactional
    public boolean tryStartTaskExecution(Long taskId, String message) {
        TaskEntity task = findTaskOrThrow(taskId);
        TaskStatus targetStatus = TaskStatus.RUNNING;
        int timeoutSeconds = task.getTimeoutSeconds() == null ? 30 : task.getTimeoutSeconds();
        LocalDateTime timeoutAt = LocalDateTime.now().plusSeconds(timeoutSeconds);
        TaskStatus claimedFromStatus = claimTaskForExecution(taskId, targetStatus, timeoutAt);

        if (claimedFromStatus == null) {
            return false;
        }

        recordTaskEvent(
                taskId,
                TaskEventType.STATUS_CHANGED,
                claimedFromStatus,
                targetStatus,
                message
        );

        return true;
    }

    private TaskStatus claimTaskForExecution(Long taskId, TaskStatus targetStatus, LocalDateTime timeoutAt) {
        if (taskStateMachine.canTransit(TaskStatus.PENDING, targetStatus)) {
            int updated = taskRepository.claimTaskForExecution(
                    taskId,
                    targetStatus,
                    timeoutAt,
                    List.of(TaskStatus.PENDING)
            );
            if (updated == 1) {
                return TaskStatus.PENDING;
            }
        }

        if (taskStateMachine.canTransit(TaskStatus.RETRY_PENDING, targetStatus)) {
            int updated = taskRepository.claimTaskForExecution(
                    taskId,
                    targetStatus,
                    timeoutAt,
                    List.of(TaskStatus.RETRY_PENDING)
            );
            if (updated == 1) {
                return TaskStatus.RETRY_PENDING;
            }
        }

        return null;
    }

    @Transactional
    public TaskDetailResponse markTaskFailed(Long taskId, String errorMessage) {
        TaskEntity task = findTaskOrThrow(taskId);
        TaskStatus currentStatus = task.getStatus();
        TaskStatus targetStatus = TaskStatus.FAILED;

        if (!taskStateMachine.canTransit(currentStatus, targetStatus)) {
            throw new ResponseStatusException(
                    HttpStatus.BAD_REQUEST,
                    "非法状态流转：" + currentStatus + " -> " + targetStatus
            );
        }

        String normalizedErrorMessage = normalizeErrorMessage(errorMessage);

        task.setStatus(targetStatus);
        task.setErrorMessage(normalizedErrorMessage);

        TaskEntity savedTask = taskRepository.save(task);

        recordTaskEvent(
                savedTask.getId(),
                TaskEventType.STATUS_CHANGED,
                currentStatus,
                targetStatus,
                normalizedErrorMessage
        );

        return toTaskDetailResponse(savedTask);
    }

    @Transactional
    public TaskDetailResponse markTaskSucceeded(
            Long taskId,
            String resultContent,
            String llmModel,
            String renderedPrompt,
            String promptTemplateCode
    ) {
        TaskEntity task = findTaskOrThrow(taskId);
        TaskStatus currentStatus = task.getStatus();
        TaskStatus targetStatus = TaskStatus.SUCCESS;

        if (!taskStateMachine.canTransit(currentStatus, targetStatus)) {
            throw new ResponseStatusException(
                    HttpStatus.BAD_REQUEST,
                    "非法状态流转：" + currentStatus + " -> " + targetStatus
            );
        }

        task.setStatus(targetStatus);
        task.setResultContent(resultContent);
        task.setLlmModel(llmModel);
        task.setRenderedPrompt(renderedPrompt);
        task.setPromptTemplateCode(promptTemplateCode);
        task.setNextRetryAt(null);
        task.setErrorMessage(null);

        TaskEntity savedTask = taskRepository.save(task);

        recordTaskEvent(
                savedTask.getId(),
                TaskEventType.STATUS_CHANGED,
                currentStatus,
                targetStatus,
                "LLM 任务执行成功"
        );

        return toTaskDetailResponse(savedTask);
    }

    @Transactional
    public void saveLlmMetadata(
            Long taskId,
            String llmProvider,
            String llmModel,
            Integer promptTokenCount,
            Integer completionTokenCount,
            Integer totalTokenCount,
            Long llmLatencyMs
    ) {
        TaskEntity task = findTaskOrThrow(taskId);
        task.setLlmProvider(llmProvider);
        task.setLlmModel(llmModel);
        task.setPromptTokenCount(promptTokenCount);
        task.setCompletionTokenCount(completionTokenCount);
        task.setTotalTokenCount(totalTokenCount);
        task.setLlmLatencyMs(llmLatencyMs);
        taskRepository.save(task);
    }

    @Transactional
    public TaskDetailResponse markTaskRetryPending(Long taskId, String errorMessage) {
        TaskEntity task = findTaskOrThrow(taskId);
        TaskStatus currentStatus = task.getStatus();
        TaskStatus targetStatus = TaskStatus.RETRY_PENDING;

        if (!taskStateMachine.canTransit(currentStatus, targetStatus)) {
            throw new ResponseStatusException(
                    HttpStatus.BAD_REQUEST,
                    "非法状态流转：" + currentStatus + " -> " + targetStatus
            );
        }

        if (task.getRetryCount() >= task.getMaxRetry()) {
            throw new ResponseStatusException(HttpStatus.BAD_REQUEST, "任务已达到最大重试次数");
        }

        String normalizedErrorMessage = normalizeErrorMessage(errorMessage);
        int nextRetryCount = task.getRetryCount() + 1;

        task.setRetryCount(nextRetryCount);
        task.setNextRetryAt(LocalDateTime.now().plusSeconds(10));
        task.setErrorMessage(normalizedErrorMessage);
        task.setStatus(targetStatus);

        TaskEntity savedTask = taskRepository.save(task);

        recordTaskEvent(
                savedTask.getId(),
                TaskEventType.STATUS_CHANGED,
                currentStatus,
                targetStatus,
                "任务执行失败，等待第 " + nextRetryCount + " 次重试：" + normalizedErrorMessage
        );

        return toTaskDetailResponse(savedTask);
    }

    @Transactional
    public TaskDetailResponse cancelTask(Long taskId, String message) {
        TaskEntity task = findTaskOrThrow(taskId);
        TaskStatus currentStatus = task.getStatus();
        TaskStatus targetStatus = TaskStatus.CANCELLED;

        if (currentStatus != TaskStatus.PENDING
                && currentStatus != TaskStatus.RETRY_PENDING
                && currentStatus != TaskStatus.RUNNING) {
            throw new ResponseStatusException(HttpStatus.BAD_REQUEST, "当前任务状态不允许取消");
        }

        if (!taskStateMachine.canTransit(currentStatus, targetStatus)) {
            throw new ResponseStatusException(
                    HttpStatus.BAD_REQUEST,
                    "非法状态流转：" + currentStatus + " -> " + targetStatus
            );
        }

        task.setStatus(targetStatus);
        task.setNextRetryAt(null);

        TaskEntity savedTask = taskRepository.save(task);

        recordTaskEvent(
                savedTask.getId(),
                TaskEventType.STATUS_CHANGED,
                currentStatus,
                targetStatus,
                message
        );

        return toTaskDetailResponse(savedTask);
    }

    @Transactional(readOnly = true)
    public boolean isTaskCancelled(Long taskId) {
        TaskEntity task = findTaskOrThrow(taskId);
        return task.getStatus() == TaskStatus.CANCELLED;
    }

    @Transactional(readOnly = true)
    public boolean isTaskRunning(Long taskId) {
        TaskEntity task = findTaskOrThrow(taskId);
        return task.getStatus() == TaskStatus.RUNNING;
    }

    @Transactional
    public TaskDetailResponse markTaskTimedOut(Long taskId) {
        TaskEntity task = findTaskOrThrow(taskId);
        TaskStatus currentStatus = task.getStatus();

        if (currentStatus != TaskStatus.RUNNING) {
            return toTaskDetailResponse(task);
        }

        TaskStatus targetStatus = TaskStatus.FAILED;

        if (!taskStateMachine.canTransit(currentStatus, targetStatus)) {
            throw new ResponseStatusException(
                    HttpStatus.BAD_REQUEST,
                    "非法状态流转：" + currentStatus + " -> " + targetStatus
            );
        }

        String errorMessage = "任务执行超时";

        task.setStatus(targetStatus);
        task.setErrorMessage(errorMessage);
        task.setNextRetryAt(null);

        TaskEntity savedTask = taskRepository.save(task);

        recordTaskEvent(
                savedTask.getId(),
                TaskEventType.STATUS_CHANGED,
                currentStatus,
                targetStatus,
                errorMessage
        );

        return toTaskDetailResponse(savedTask);
    }

    private String normalizeErrorMessage(String errorMessage) {
        String normalizedErrorMessage = errorMessage;

        if (normalizedErrorMessage == null || normalizedErrorMessage.isBlank()) {
            normalizedErrorMessage = DEFAULT_ERROR_MESSAGE;
        }

        if (normalizedErrorMessage.length() > MAX_ERROR_MESSAGE_LENGTH) {
            return normalizedErrorMessage.substring(0, MAX_ERROR_MESSAGE_LENGTH);
        }

        return normalizedErrorMessage;
    }

    private void recordTaskEvent(
            Long taskId,
            TaskEventType eventType,
            TaskStatus fromStatus,
            TaskStatus toStatus,
            String message
    ) {
        TaskEventEntity event = new TaskEventEntity();
        event.setTaskId(taskId);
        event.setEventType(eventType);
        event.setFromStatus(fromStatus);
        event.setToStatus(toStatus);
        event.setMessage(message);

        taskEventRepository.save(event);
    }

    private TaskEntity findTaskOrThrow(Long taskId) {
        return taskRepository.findById(taskId)
                .orElseThrow(() -> new ResponseStatusException(HttpStatus.NOT_FOUND, "任务不存在"));
    }

    private TaskDetailResponse toTaskDetailResponse(TaskEntity task) {
        return new TaskDetailResponse(
                task.getId(),
                task.getPrompt(),
                task.getRequestedModel(),
                task.getStatus(),
                task.getErrorMessage(),
                task.getRetryCount(),
                task.getMaxRetry(),
                task.getNextRetryAt(),
                task.getTimeoutSeconds(),
                task.getTimeoutAt(),
                task.getResultContent(),
                task.getLlmModel(),
                task.getRenderedPrompt(),
                task.getPromptTemplateCode(),
                task.getLlmProvider(),
                task.getPromptTokenCount(),
                task.getCompletionTokenCount(),
                task.getTotalTokenCount(),
                task.getLlmLatencyMs(),
                task.getCreatedAt(),
                task.getUpdatedAt()
        );
    }
    private final TaskDispatchProducer taskDispatchProducer;
}
