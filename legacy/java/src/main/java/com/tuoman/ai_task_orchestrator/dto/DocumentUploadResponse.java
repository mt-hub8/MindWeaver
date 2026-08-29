package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class DocumentUploadResponse {

    private Long documentId;

    private String originalFilename;

    private String status;

    private Integer chunkCount;
}
