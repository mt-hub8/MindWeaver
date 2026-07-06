package com.tuoman.ai_task_orchestrator.document;

import com.tuoman.ai_task_orchestrator.enums.ChunkType;
import lombok.Getter;

@Getter
public class StructuredChunkResult {

    private final Integer chunkIndex;

    private final String content;

    private final Integer contentLength;

    private final String chunkStrategy;

    private final Integer startOffset;

    private final Integer endOffset;

    private final String headingPath;

    private final String sectionPath;

    private final String sectionTitle;

    private final Integer headingLevel;

    private final ChunkType chunkType;

    private final String contentHash;

    private final String normalizedContentHash;

    private final Integer tokenCount;

    private final String language;

    private final Integer previousChunkIndex;

    private final Integer parentChunkIndex;

    public StructuredChunkResult(
            Integer chunkIndex,
            String content,
            Integer contentLength,
            String chunkStrategy,
            Integer startOffset,
            Integer endOffset,
            String headingPath,
            String sectionPath,
            String sectionTitle,
            Integer headingLevel,
            ChunkType chunkType,
            String contentHash,
            String normalizedContentHash,
            Integer tokenCount,
            String language,
            Integer previousChunkIndex,
            Integer parentChunkIndex
    ) {
        this.chunkIndex = chunkIndex;
        this.content = content;
        this.contentLength = contentLength;
        this.chunkStrategy = chunkStrategy;
        this.startOffset = startOffset;
        this.endOffset = endOffset;
        this.headingPath = headingPath;
        this.sectionPath = sectionPath;
        this.sectionTitle = sectionTitle;
        this.headingLevel = headingLevel;
        this.chunkType = chunkType;
        this.contentHash = contentHash;
        this.normalizedContentHash = normalizedContentHash;
        this.tokenCount = tokenCount;
        this.language = language;
        this.previousChunkIndex = previousChunkIndex;
        this.parentChunkIndex = parentChunkIndex;
    }

    public DocumentChunkResult toLegacyResult() {
        return new DocumentChunkResult(
                chunkIndex,
                content,
                contentLength,
                chunkStrategy,
                startOffset,
                endOffset,
                headingPath != null ? headingPath : sectionPath
        );
    }
}
