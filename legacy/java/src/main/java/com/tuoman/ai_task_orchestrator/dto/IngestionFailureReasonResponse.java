package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class IngestionFailureReasonResponse {

    private String errorCode;

    private String displayMessage;

    private Integer count;
}
