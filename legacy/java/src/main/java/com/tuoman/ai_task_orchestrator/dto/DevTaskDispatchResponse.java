package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class DevTaskDispatchResponse {

    private Long taskId;

    private boolean dispatched;

    private String message;
}
