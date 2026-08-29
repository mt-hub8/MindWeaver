package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.time.LocalDateTime;

@Getter
@AllArgsConstructor
public class TaskOutputChunkResponse {

    private Long id;

    private Long taskId;

    private Integer chunkIndex;

    private String content;

    private LocalDateTime createdAt;
}
