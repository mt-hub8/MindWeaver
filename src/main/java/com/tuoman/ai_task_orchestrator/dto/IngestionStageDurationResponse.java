package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class IngestionStageDurationResponse {

    private String stage;

    private String displayName;

    private Long averageDurationMs;

    private Integer sampleCount;
}
