package com.tuoman.ai_task_orchestrator.dto;

import com.tuoman.ai_task_orchestrator.enums.MemoryScope;
import com.tuoman.ai_task_orchestrator.enums.MemorySourceType;
import com.tuoman.ai_task_orchestrator.enums.MemoryStatus;
import com.tuoman.ai_task_orchestrator.enums.MemoryType;
import com.tuoman.ai_task_orchestrator.enums.MemoryVisibility;
import lombok.Builder;
import lombok.Getter;

import java.time.LocalDateTime;

@Getter
@Builder
public class MemoryResponse {

    private Long id;
    private String memoryKey;
    private String title;
    private String content;
    private MemoryType memoryType;
    private MemoryScope memoryScope;
    private MemoryVisibility visibility;
    private MemoryStatus status;
    private MemorySourceType sourceType;
    private String sourceId;
    private Long projectId;
    private Long agentProfileId;
    private Long taskId;
    private Double confidence;
    private Integer importance;
    private LocalDateTime expiresAt;
    private LocalDateTime lastUsedAt;
    private Long useCount;
    private LocalDateTime createdAt;
    private LocalDateTime updatedAt;
    private LocalDateTime deletedAt;
    private String metadataJson;
}
