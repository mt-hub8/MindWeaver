package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class CreateAgentTaskResponse {

    private Long taskId;

    private String status;

    private String displayStatus;

    private String displayMessage;
}
