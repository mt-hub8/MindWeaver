package com.tuoman.ai_task_orchestrator.retrieval;

import java.util.List;

public interface KeywordRetriever {

    KeywordRetrievalResponse search(String query, RetrievalFilter filter, int topK);

    String name();

    record KeywordCandidate(
            int rank,
            Long documentId,
            String documentTitle,
            Long chunkId,
            String sectionPath,
            String content,
            double score,
            String matchReason
    ) {
    }

    record KeywordRetrievalResponse(List<KeywordCandidate> candidates, long latencyMs) {
    }
}
