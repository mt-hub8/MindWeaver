package com.tuoman.ai_task_orchestrator.retrieval;

import com.tuoman.ai_task_orchestrator.enums.RetrievalScopeType;

import java.util.Collections;
import java.util.Set;

public record RetrievalScope(
        RetrievalScopeType scopeType,
        Long collectionId,
        String collectionName,
        Set<Long> allowedDocumentIds
) {

    public static RetrievalScope allDocuments() {
        return new RetrievalScope(RetrievalScopeType.ALL_DOCUMENTS, null, null, null);
    }

    public static RetrievalScope collection(Long collectionId, String collectionName, Set<Long> allowedDocumentIds) {
        return new RetrievalScope(
                RetrievalScopeType.COLLECTION,
                collectionId,
                collectionName,
                allowedDocumentIds == null ? Set.of() : Set.copyOf(allowedDocumentIds)
        );
    }

    public boolean isCollectionScoped() {
        return scopeType == RetrievalScopeType.COLLECTION;
    }

    public Set<Long> allowedDocumentIdsOrEmpty() {
        return allowedDocumentIds == null ? Collections.emptySet() : allowedDocumentIds;
    }
}
