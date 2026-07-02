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

    private LocalDateTime createdAt;

    private LocalDateTime updatedAt;
}
