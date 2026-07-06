package com.tuoman.ai_task_orchestrator.entity;

import com.tuoman.ai_task_orchestrator.kbhealth.RagEvaluationRetrievalStrategy;
import com.tuoman.ai_task_orchestrator.kbhealth.RagEvaluationRunStatus;
import com.tuoman.ai_task_orchestrator.kbhealth.RagHealthScoringProfile;
import jakarta.persistence.*;
import lombok.Getter;
import lombok.Setter;

import java.time.LocalDateTime;

@Getter
@Setter
@Entity
@Table(name = "rag_evaluation_run")
public class RagEvaluationRunEntity {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @Column(name = "dataset_id", nullable = false)
    private Long datasetId;

    @Column(nullable = false, length = 255)
    private String name;

    @Enumerated(EnumType.STRING)
    @Column(nullable = false, length = 32)
    private RagEvaluationRunStatus status;

    @Enumerated(EnumType.STRING)
    @Column(nullable = false, length = 64)
    private RagEvaluationRetrievalStrategy strategy;

    @Enumerated(EnumType.STRING)
    @Column(name = "scoring_profile", nullable = false, length = 64)
    private RagHealthScoringProfile scoringProfile;

    @Column(name = "top_k", nullable = false)
    private Integer topK;

    @Column(name = "retrieval_top_k")
    private Integer retrievalTopK;

    @Column(name = "rerank_top_n")
    private Integer rerankTopN;

    @Column(name = "collection_id")
    private Long collectionId;

    @Column(name = "metadata_filter_json", columnDefinition = "TEXT")
    private String metadataFilterJson;

    @Column(name = "execute_generation", nullable = false)
    private Boolean executeGeneration;

    @Column(name = "started_at")
    private LocalDateTime startedAt;

    @Column(name = "completed_at")
    private LocalDateTime completedAt;

    @Column(name = "created_at", nullable = false)
    private LocalDateTime createdAt;

    @Column(name = "updated_at", nullable = false)
    private LocalDateTime updatedAt;

    @Column(name = "total_cases", nullable = false)
    private Integer totalCases;

    @Column(name = "completed_cases", nullable = false)
    private Integer completedCases;

    @Column(name = "failed_cases", nullable = false)
    private Integer failedCases;

    @Column(name = "overall_score")
    private Integer overallScore;

    @Column(name = "summary_json", columnDefinition = "MEDIUMTEXT")
    private String summaryJson;

    @Column(name = "diagnosis_json", columnDefinition = "MEDIUMTEXT")
    private String diagnosisJson;

    @PrePersist
    public void prePersist() {
        LocalDateTime now = LocalDateTime.now();
        this.createdAt = now;
        this.updatedAt = now;
        if (this.status == null) {
            this.status = RagEvaluationRunStatus.CREATED;
        }
        if (this.scoringProfile == null) {
            this.scoringProfile = RagHealthScoringProfile.BALANCED;
        }
        if (this.executeGeneration == null) {
            this.executeGeneration = false;
        }
        initCounts();
    }

    @PreUpdate
    public void preUpdate() {
        this.updatedAt = LocalDateTime.now();
    }

    public void initCounts() {
        if (totalCases == null) {
            totalCases = 0;
        }
        if (completedCases == null) {
            completedCases = 0;
        }
        if (failedCases == null) {
            failedCases = 0;
        }
    }
}
