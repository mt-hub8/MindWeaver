package com.tuoman.ai_task_orchestrator.dto;

import com.fasterxml.jackson.annotation.JsonInclude;
import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
@JsonInclude(JsonInclude.Include.NON_NULL)
public class RagRetrievalMetadataResponse {

    private Integer topK;

    private Integer returned;

    private String provider;

    private String model;

    private Integer dimension;

    private String vectorStore;

    private Boolean rerankEnabled;

    private String rerankerName;

    private Integer candidateTopK;

    private Integer finalTopK;

    private Long rerankLatencyMs;

    private Boolean hybridEnabled;

    private Integer denseTopK;

    private Integer lexicalTopK;

    private String fusionStrategy;

    private Integer denseCandidateCount;

    private Integer lexicalCandidateCount;

    private Integer fusedCandidateCount;

    private Long hybridLatencyMs;

    private String scopeType;

    private Long collectionId;

    private String collectionName;

    private Integer filteredByCollectionCount;

    private Integer filteredByLifecycleCount;

    private Integer finalContextCount;

    public RagRetrievalMetadataResponse(
            Integer topK,
            Integer returned,
            String provider,
            String model,
            Integer dimension,
            String vectorStore
    ) {
        this(
                topK,
                returned,
                provider,
                model,
                dimension,
                vectorStore,
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                null
        );
    }
}
