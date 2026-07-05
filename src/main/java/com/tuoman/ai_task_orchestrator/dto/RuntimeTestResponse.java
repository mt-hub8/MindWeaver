package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class RuntimeTestResponse {

    private boolean success;

    private String message;

    private Long latencyMs;

    private String provider;

    private String model;
}
