package com.tuoman.ai_task_orchestrator.common.error;

import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class ApiErrorResponse {

    private final String timestamp;

    private final int status;

    private final String code;

    private final String message;

    private final String path;

    private final String traceId;
}
