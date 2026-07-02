package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.time.LocalDateTime;
import java.util.List;

@Getter
@AllArgsConstructor
public class AgentTaskDetailResponse {

    private Long taskId;

    private String title;

    private String objective;

    private String status;

    private String displayStatus;

    private Long collectionId;

    private String collectionName;

    private String scopeLabel;

    private String result;

    private String errorCode;

    private String errorMessage;

    private String traceId;

    private AgentTaskModelMetadataResponse modelMetadata;

    private List<AgentTaskCitationResponse> citations;

    private List<AgentTaskStepResponse> steps;

    private Integer stepCount;

    private Integer toolExecutionCount;

    private Integer failedStepCount;

    private Long finalReportLatencyMs;

    private LocalDateTime createdAt;

    private LocalDateTime startedAt;

    private LocalDateTime completedAt;
}
