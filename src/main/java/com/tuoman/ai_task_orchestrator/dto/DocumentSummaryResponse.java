package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.time.LocalDateTime;

@Getter
@AllArgsConstructor
public class DocumentSummaryResponse {

    private Long documentId;

    private String title;

    private Integer chunkCount;

    private String status;

    private String displayStatus;

    private String processingStatus;

    private LocalDateTime deletedAt;

    private Boolean canDelete;

    private Boolean canAsk;

    private Integer currentGeneration;

    private Integer reindexCount;

    private LocalDateTime lastReindexedAt;

    private Boolean canReindex;

    private String reindexDisabledReason;

    private LocalDateTime createdAt;

    private LocalDateTime updatedAt;
}
