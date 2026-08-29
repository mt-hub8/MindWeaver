package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.time.LocalDateTime;
import java.util.List;

@Getter
@AllArgsConstructor
public class CollectionDetailResponse {

    private Long collectionId;

    private String name;

    private String description;

    private Integer documentCount;

    private Integer activeDocumentCount;

    private LocalDateTime createdAt;

    private LocalDateTime updatedAt;

    private List<CollectionDocumentItemResponse> documents;
}
