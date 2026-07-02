package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.time.LocalDateTime;

@Getter
@AllArgsConstructor
public class AgentTaskSummaryResponse {

    private Long taskId;

    private String title;

    private String status;

    private String displayStatus;

    private Long collectionId;

    private String collectionName;

    private Integer citationCount;

    private LocalDateTime createdAt;

    private LocalDateTime completedAt;
}
