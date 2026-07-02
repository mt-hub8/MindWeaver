package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.dto.AgentTaskDetailResponse;
import com.tuoman.ai_task_orchestrator.dto.AgentTaskEventResponse;
import com.tuoman.ai_task_orchestrator.dto.AgentTaskSummaryResponse;
import com.tuoman.ai_task_orchestrator.dto.CreateAgentTaskRequest;
import com.tuoman.ai_task_orchestrator.dto.CreateAgentTaskResponse;
import com.tuoman.ai_task_orchestrator.service.AgentTaskQueryService;
import com.tuoman.ai_task_orchestrator.service.AgentTaskService;
import jakarta.validation.Valid;
import lombok.RequiredArgsConstructor;
import org.springframework.http.HttpStatus;
import org.springframework.web.bind.annotation.*;

import java.util.List;

@RestController
@RequestMapping("/agent/tasks")
@RequiredArgsConstructor
public class AgentTaskController {

    private final AgentTaskService agentTaskService;

    private final AgentTaskQueryService agentTaskQueryService;

    @PostMapping
    @ResponseStatus(HttpStatus.ACCEPTED)
    public CreateAgentTaskResponse createTask(@Valid @RequestBody CreateAgentTaskRequest request) {
        return agentTaskService.createTask(request);
    }

    @GetMapping
    public List<AgentTaskSummaryResponse> listTasks() {
        return agentTaskQueryService.listTasks();
    }

    @GetMapping("/{taskId}")
    public AgentTaskDetailResponse getTask(@PathVariable Long taskId) {
        return agentTaskQueryService.getTask(taskId);
    }

    @GetMapping("/{taskId}/events")
    public List<AgentTaskEventResponse> listEvents(@PathVariable Long taskId) {
        return agentTaskQueryService.listEvents(taskId);
    }
}
