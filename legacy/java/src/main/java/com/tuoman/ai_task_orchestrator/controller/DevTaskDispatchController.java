package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.dto.DevTaskDispatchResponse;
import com.tuoman.ai_task_orchestrator.mq.TaskDispatchProducer;
import lombok.RequiredArgsConstructor;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/dev/tasks")
@RequiredArgsConstructor
public class DevTaskDispatchController {

    private final TaskDispatchProducer taskDispatchProducer;

    @PostMapping("/{taskId}/dispatch")
    public DevTaskDispatchResponse dispatchTask(@PathVariable Long taskId) {
        taskDispatchProducer.sendTaskCreatedMessage(taskId);
        return new DevTaskDispatchResponse(taskId, true, "Task dispatch message sent");
    }
}
