package com.tuoman.ai_task_orchestrator.dto;

import com.fasterxml.jackson.annotation.JsonInclude;
import lombok.Getter;

@Getter
@JsonInclude(JsonInclude.Include.NON_NULL)
public class RagCitationResponse {

    private Integer sourceIndex;

    private Long documentId;

    private Long chunkId;

    private Double score;

    private String contentSnippet;

    private Integer originalRank;

    private Integer rerankedRank;

    private Double originalScore;

    private Double rerankScore;

    private Integer denseRank;

    private Integer lexicalRank;

    private Double denseScore;

    private Double lexicalScore;

    private Double fusionScore;

    private Boolean denseHit;

    private Boolean lexicalHit;

    private String documentTitle;

    private String sectionPath;

    private String version;

    private String citationKey;

    private String supportLevel;

    public RagCitationResponse(
            Integer sourceIndex,
            Long documentId,
            Long chunkId,
            Double score,
            String contentSnippet
    ) {
        this(
                sourceIndex,
                documentId,
                chunkId,
                score,
                contentSnippet,
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

    public RagCitationResponse(
            Integer sourceIndex,
            Long documentId,
            Long chunkId,
            Double score,
            String contentSnippet,
            Integer originalRank,
            Integer rerankedRank,
            Double originalScore,
            Double rerankScore,
            Integer denseRank,
            Integer lexicalRank,
            Double denseScore,
            Double lexicalScore,
            Double fusionScore,
            Boolean denseHit,
            Boolean lexicalHit
    ) {
        this(
                sourceIndex,
                documentId,
                chunkId,
                score,
                contentSnippet,
                originalRank,
                rerankedRank,
                originalScore,
                rerankScore,
                denseRank,
                lexicalRank,
                denseScore,
                lexicalScore,
                fusionScore,
                denseHit,
                lexicalHit,
                null,
                null,
                null,
                null,
                null
        );
    }

    public RagCitationResponse(
            Integer sourceIndex,
            Long documentId,
            Long chunkId,
            Double score,
            String contentSnippet,
            Integer originalRank,
            Integer rerankedRank,
            Double originalScore,
            Double rerankScore
    ) {
        this(
                sourceIndex,
                documentId,
                chunkId,
                score,
                contentSnippet,
                originalRank,
                rerankedRank,
                originalScore,
                rerankScore,
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

    public RagCitationResponse(
            Integer sourceIndex,
            Long documentId,
            Long chunkId,
            Double score,
            String contentSnippet,
            Integer originalRank,
            Integer rerankedRank,
            Double originalScore,
            Double rerankScore,
            Integer denseRank,
            Integer lexicalRank,
            Double denseScore,
            Double lexicalScore,
            Double fusionScore,
            Boolean denseHit,
            Boolean lexicalHit,
            String documentTitle,
            String sectionPath,
            String version,
            String citationKey,
            String supportLevel
    ) {
        this.sourceIndex = sourceIndex;
        this.documentId = documentId;
        this.chunkId = chunkId;
        this.score = score;
        this.contentSnippet = contentSnippet;
        this.originalRank = originalRank;
        this.rerankedRank = rerankedRank;
        this.originalScore = originalScore;
        this.rerankScore = rerankScore;
        this.denseRank = denseRank;
        this.lexicalRank = lexicalRank;
        this.denseScore = denseScore;
        this.lexicalScore = lexicalScore;
        this.fusionScore = fusionScore;
        this.denseHit = denseHit;
        this.lexicalHit = lexicalHit;
        this.documentTitle = documentTitle;
        this.sectionPath = sectionPath;
        this.version = version;
        this.citationKey = citationKey;
        this.supportLevel = supportLevel;
    }
}
