package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class DocumentSearchResultResponse {

    private Long documentId;

    private Long chunkId;

    private Integer chunkIndex;

    private Double score;

    private String content;

    private Integer contentLength;

    private String headingPath;

    private Integer startOffset;

    private Integer endOffset;

    private String chunkStrategy;

    private String embeddingProvider;

    private String embeddingModel;

    private String distanceMetric;
}
