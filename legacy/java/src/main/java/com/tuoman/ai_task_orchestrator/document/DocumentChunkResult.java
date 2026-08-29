package com.tuoman.ai_task_orchestrator.document;

import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class DocumentChunkResult {

    private Integer chunkIndex;

    private String content;

    private Integer contentLength;

    private String chunkStrategy;

    private Integer startOffset;

    private Integer endOffset;

    private String headingPath;
}
