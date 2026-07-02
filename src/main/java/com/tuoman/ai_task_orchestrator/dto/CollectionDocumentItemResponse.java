package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class CollectionDocumentItemResponse {

    private Long documentId;

    private String title;

    private String status;

    private String displayStatus;

    private Boolean canAsk;

    private Boolean canRemoveFromCollection;

    private String lifecycleHint;
}
