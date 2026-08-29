package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.time.LocalDateTime;

@Getter
@AllArgsConstructor
public class DocumentDetailResponse {

    private Long id;

    private String originalFilename;

    private String contentType;

    private Long fileSize;

    private String status;

    private Integer chunkCount;

    private String errorMessage;

    private LocalDateTime createdAt;

    private LocalDateTime updatedAt;
}
