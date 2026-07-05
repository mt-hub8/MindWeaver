package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class CacheClearResponse {

    private boolean success;

    private int clearedCount;

    private Long clearedBytes;

    private String message;
}
