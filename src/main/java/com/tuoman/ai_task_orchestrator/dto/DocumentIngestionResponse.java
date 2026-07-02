package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class DocumentIngestionResponse {

    private Long documentId;

    private String title;

    private String status;

    private Integer chunkCount;

    private Integer embeddingCount;

    private Integer vectorWriteCount;
}
