package com.tuoman.ai_task_orchestrator.hybrid;

import java.util.List;
import java.util.Set;

public record LexicalRetrievalRequest(
        String query,
        int lexicalTopK,
        Long documentId,
        List<Long> scopedDocumentIds
) {
    public LexicalRetrievalRequest(String query, int lexicalTopK, Long documentId) {
        this(query, lexicalTopK, documentId, null);
    }

    public static LexicalRetrievalRequest forScope(String query, int lexicalTopK, Set<Long> scopedDocumentIds) {
        if (scopedDocumentIds == null || scopedDocumentIds.isEmpty()) {
            return new LexicalRetrievalRequest(query, lexicalTopK, null, List.of());
        }
        return new LexicalRetrievalRequest(query, lexicalTopK, null, scopedDocumentIds.stream().toList());
    }
}
