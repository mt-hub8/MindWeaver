package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.time.LocalDateTime;
import java.util.List;

@Getter
@AllArgsConstructor
public class DocumentSummaryResponse {

    private Long documentId;

    private String title;

    private Integer chunkCount;

    private String status;

    private String displayStatus;

    private String processingStatus;

    private String displayProcessingStatus;

    private String lifecycleHint;

    private LocalDateTime deletedAt;

    private Boolean canDelete;

    private Boolean canAsk;

    private Integer currentGeneration;

    private Integer reindexCount;

    private LocalDateTime lastReindexedAt;

    private Boolean canReindex;

    private String reindexDisabledReason;

    private List<CollectionMembershipResponse> collections;

    private List<Long> collectionIds;

    private List<String> collectionNames;

    private Boolean canAssignToCollection;

    private Boolean canRemoveFromCollection;

    private LocalDateTime createdAt;

    private LocalDateTime updatedAt;
}
