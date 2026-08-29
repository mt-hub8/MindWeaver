package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.time.LocalDateTime;

@Getter
@AllArgsConstructor
public class DocumentChunkResponse {

    private Long id;

    private Long documentId;

    private Integer chunkIndex;

    private String content;

    private Integer contentLength;

    private String chunkStrategy;

    private Integer startOffset;

    private Integer endOffset;

    private String headingPath;

    private LocalDateTime createdAt;
}
