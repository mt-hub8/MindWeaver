package com.tuoman.ai_task_orchestrator.dto;

import com.tuoman.ai_task_orchestrator.enums.TaskStatus;
import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class CreateTaskResponse {

    private Long taskId;

    private TaskStatus status;

}