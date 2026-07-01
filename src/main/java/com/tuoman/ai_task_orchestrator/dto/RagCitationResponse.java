package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class RagCitationResponse {

    private Integer citationId;

    private Long documentId;

    private Long chunkId;

    private Integer chunkIndex;

    private Double score;

    private String headingPath;

    private Integer startOffset;

    private Integer endOffset;

    private String chunkStrategy;

    private String contentPreview;
}
