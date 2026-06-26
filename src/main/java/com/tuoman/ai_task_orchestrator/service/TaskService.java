package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.CreateTaskRequest;
import com.tuoman.ai_task_orchestrator.dto.CreateTaskResponse;
import com.tuoman.ai_task_orchestrator.dto.TaskDetailResponse;
import com.tuoman.ai_task_orchestrator.entity.TaskEntity;
import com.tuoman.ai_task_orchestrator.enums.TaskStatus;
import com.tuoman.ai_task_orchestrator.repository.TaskRepository;
import com.tuoman.ai_task_orchestrator.state.TaskStateMachine;
import lombok.RequiredArgsConstructor;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;
import org.springframework.web.server.ResponseStatusException;

@Service
@RequiredArgsConstructor
public class TaskService {

    private final TaskRepository taskRepository;

    private final TaskStateMachine taskStateMachine;

    @Transactional
    public CreateTaskResponse createTask(CreateTaskRequest request) {
        TaskEntity task = new TaskEntity();
        task.setPrompt(request.getPrompt());
        task.setStatus(TaskStatus.PENDING);

        TaskEntity savedTask = taskRepository.save(task);

        return new CreateTaskResponse(savedTask.getId(), savedTask.getStatus());
    }

    @Transactional(readOnly = true)
    public TaskDetailResponse getTaskById(Long taskId) {
        TaskEntity task = findTaskOrThrow(taskId);
        return toTaskDetailResponse(task);
    }

    @Transactional
    public TaskDetailResponse updateTaskStatus(Long taskId, TaskStatus targetStatus) {
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

        return toTaskDetailResponse(savedTask);
    }

    private TaskEntity findTaskOrThrow(Long taskId) {
        return taskRepository.findById(taskId)
                .orElseThrow(() -> new ResponseStatusException(HttpStatus.NOT_FOUND, "任务不存在"));
    }

    private TaskDetailResponse toTaskDetailResponse(TaskEntity task) {
        return new TaskDetailResponse(
                task.getId(),
                task.getPrompt(),
                task.getStatus(),
                task.getCreatedAt(),
                task.getUpdatedAt()
        );
    }
}