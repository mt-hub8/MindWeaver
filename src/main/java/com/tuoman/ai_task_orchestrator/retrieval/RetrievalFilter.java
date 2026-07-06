package com.tuoman.ai_task_orchestrator.retrieval;

import com.tuoman.ai_task_orchestrator.enums.ChunkMetadataStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentDocType;
import lombok.Builder;
import lombok.Getter;

import java.time.LocalDateTime;
import java.util.List;
import java.util.Set;

@Getter
@Builder
public class RetrievalFilter {

    private final Long collectionId;

    private final DocumentDocType docType;

    private final String version;

    private final String source;

    private final ChunkMetadataStatus status;

    private final List<String> tags;

    private final LocalDateTime createdAfter;

    private final LocalDateTime createdBefore;

    private final boolean includeDeprecated;

    private final boolean includeDraft;

    private final boolean includeTrashed;

    private final Set<Long> scopedDocumentIds;

    public static RetrievalFilter empty() {
        return RetrievalFilter.builder().build();
    }
}
