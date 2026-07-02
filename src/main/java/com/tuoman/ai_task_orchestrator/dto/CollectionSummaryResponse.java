package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.time.LocalDateTime;

@Getter
@AllArgsConstructor
public class CollectionSummaryResponse {

    private Long collectionId;

    private String name;

    private String description;

    private Integer documentCount;

    private Integer activeDocumentCount;

    private LocalDateTime createdAt;

    private LocalDateTime updatedAt;
}
