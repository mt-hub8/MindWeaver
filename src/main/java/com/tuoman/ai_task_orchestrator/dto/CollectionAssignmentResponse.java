package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class CollectionAssignmentResponse {

    private Long collectionId;

    private Long documentId;

    private String message;
}
