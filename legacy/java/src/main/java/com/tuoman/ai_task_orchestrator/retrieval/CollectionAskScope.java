package com.tuoman.ai_task_orchestrator.retrieval;

import java.util.Set;

public record CollectionAskScope(
        Long collectionId,
        String collectionName,
        CollectionAskEmptyReason emptyReason,
        Set<Long> memberDocumentIds,
        Set<Long> askableDocumentIds,
        String noContextMessage
) {

    public static CollectionAskScope notApplicable() {
        return new CollectionAskScope(null, null, CollectionAskEmptyReason.NONE, Set.of(), Set.of(), null);
    }

    public boolean shouldSkipRetrieval() {
        return emptyReason != null && emptyReason != CollectionAskEmptyReason.NONE;
    }
}
