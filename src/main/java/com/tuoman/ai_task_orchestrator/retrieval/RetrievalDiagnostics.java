package com.tuoman.ai_task_orchestrator.retrieval;

import lombok.Builder;
import lombok.Getter;

import java.util.List;
import java.util.Map;

@Getter
@Builder
public class RetrievalDiagnostics {

    private final String query;

    private final String strategy;

    private final RetrievalFilter filter;

    private final String filterMode;

    private final int vectorTopK;

    private final int keywordTopK;

    private final int finalTopK;

    private final String fusionStrategy;

    private final boolean rerankerEnabled;

    private final String contextExpansion;

    private final int candidateCount;

    private final int vectorHitCount;

    private final int keywordHitCount;

    private final int overlapCount;

    private final int filteredOutCount;

    private final int crossCollectionLeakCount;

    private final int wrongVersionLeakCount;

    private final int deprecatedHitCount;

    private final List<FinalChunkDiagnostic> finalChunks;

    private final List<String> warnings;

    @Getter
    @Builder
    public static class FinalChunkDiagnostic {
        private final Long chunkId;
        private final Long documentId;
        private final String documentTitle;
        private final Long collectionId;
        private final String version;
        private final String docType;
        private final String sectionPath;
        private final Double score;
        private final int rank;
        private final String source;
        private final String whySelected;
        private final boolean wasExpanded;
    }
}
