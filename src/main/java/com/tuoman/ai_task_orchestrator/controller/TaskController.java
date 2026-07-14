package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.dto.CreateTaskRequest;
import com.tuoman.ai_task_orchestrator.dto.CreateTaskResponse;
import com.tuoman.ai_task_orchestrator.dto.TaskAttemptResponse;
import com.tuoman.ai_task_orchestrator.dto.TaskDetailResponse;
import com.tuoman.ai_task_orchestrator.dto.TaskOutputChunkResponse;
import com.tuoman.ai_task_orchestrator.service.TaskAttemptService;
import com.tuoman.ai_task_orchestrator.service.TaskOutputChunkService;
import com.tuoman.ai_task_orchestrator.service.TaskService;
import jakarta.validation.Valid;
import lombok.RequiredArgsConstructor;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.util.List;

@RestController
@RequestMapping("/tasks")
@RequiredArgsConstructor
/**
 * V0.x Task 编排 HTTP 入口。
 *
 * V0.1/V0.2 从创建和查询任务开始，后续版本在同一资源上扩展取消、attempt
 * 和 output chunks。Controller 只暴露用户可见 API，状态机、事件、MQ 派发
 * 和 LLM 执行都由 service/worker 层维护。
 */
public class TaskController {

    private final TaskService taskService;

    private final TaskOutputChunkService taskOutputChunkService;

    private final TaskAttemptService taskAttemptService;

    @PostMapping
    public CreateTaskResponse createTask(@Valid @RequestBody CreateTaskRequest request) {
        return taskService.createTask(request);
    }

    @GetMapping("/{taskId}")
    public TaskDetailResponse getTaskById(@PathVariable Long taskId) {
        return taskService.getTaskById(taskId);
    }

    @PostMapping("/{taskId}/cancel")
    public TaskDetailResponse cancelTask(@PathVariable Long taskId) {
        return taskService.cancelTask(taskId, "任务已取消");
    }

    @GetMapping("/{taskId}/output-chunks")
    public List<TaskOutputChunkResponse> getOutputChunks(@PathVariable Long taskId) {
        return taskOutputChunkService.getChunks(taskId);
    }

    @GetMapping("/{taskId}/attempts")
    public List<TaskAttemptResponse> getAttempts(@PathVariable Long taskId) {
        return taskAttemptService.getAttempts(taskId);
    }
}
