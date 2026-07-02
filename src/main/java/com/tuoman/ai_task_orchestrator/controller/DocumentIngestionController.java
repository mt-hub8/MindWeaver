package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.dto.DocumentIngestionEventTimelineResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentIngestionTaskResponse;
import com.tuoman.ai_task_orchestrator.service.DocumentIngestionEventService;
import com.tuoman.ai_task_orchestrator.service.DocumentIngestionTaskService;
import lombok.RequiredArgsConstructor;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.util.List;

@RestController
@RequestMapping("/documents/ingestions")
@RequiredArgsConstructor
public class DocumentIngestionController {

    private final DocumentIngestionTaskService documentIngestionTaskService;

    private final DocumentIngestionEventService documentIngestionEventService;

    @GetMapping("/{taskId}")
    public DocumentIngestionTaskResponse getTask(@PathVariable Long taskId) {
        return documentIngestionTaskService.getTask(taskId);
    }

    @GetMapping
    public List<DocumentIngestionTaskResponse> listRecentTasks() {
        return documentIngestionTaskService.listRecentTasks();
    }

    @PostMapping("/{taskId}/retry")
    public DocumentIngestionTaskResponse retryTask(@PathVariable Long taskId) {
        return documentIngestionTaskService.retryTask(taskId);
    }

    @GetMapping("/{taskId}/events")
    public DocumentIngestionEventTimelineResponse getTaskEvents(@PathVariable Long taskId) {
        return documentIngestionEventService.getEventTimeline(taskId);
    }
}
