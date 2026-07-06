package com.tuoman.ai_task_orchestrator.entity;

import com.tuoman.ai_task_orchestrator.kbhealth.RagEvaluationQueryType;
import jakarta.persistence.*;
import lombok.Getter;
import lombok.Setter;

import java.time.LocalDateTime;

@Getter
@Setter
@Entity
@Table(name = "rag_evaluation_case")
public class RagEvaluationCaseEntity {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @Column(name = "dataset_id", nullable = false)
    private Long datasetId;

    @Column(name = "case_id", nullable = false, length = 128)
    private String caseId;

    @Column(nullable = false, columnDefinition = "TEXT")
    private String query;

    @Enumerated(EnumType.STRING)
    @Column(name = "query_type", length = 64)
    private RagEvaluationQueryType queryType;

    @Column(name = "collection_id")
    private Long collectionId;

    @Column(name = "expected_doc_ids_json", columnDefinition = "TEXT")
    private String expectedDocIdsJson;

    @Column(name = "expected_chunk_ids_json", columnDefinition = "TEXT")
    private String expectedChunkIdsJson;

    @Column(name = "expected_rank_json", columnDefinition = "TEXT")
    private String expectedRankJson;

    @Column(name = "negative_doc_ids_json", columnDefinition = "TEXT")
    private String negativeDocIdsJson;

    @Column(name = "expected_answer_points_json", columnDefinition = "TEXT")
    private String expectedAnswerPointsJson;

    @Column(name = "answer_must_cite", nullable = false)
    private Boolean answerMustCite;

    @Column(name = "metadata_filter_json", columnDefinition = "TEXT")
    private String metadataFilterJson;

    @Column(length = 32)
    private String difficulty;

    @Column(name = "tags_json", columnDefinition = "TEXT")
    private String tagsJson;

    @Column(nullable = false)
    private Boolean enabled;

    @Column(name = "created_at", nullable = false)
    private LocalDateTime createdAt;

    @Column(name = "updated_at", nullable = false)
    private LocalDateTime updatedAt;

    @PrePersist
    public void prePersist() {
        LocalDateTime now = LocalDateTime.now();
        this.createdAt = now;
        this.updatedAt = now;
        if (this.enabled == null) {
            this.enabled = true;
        }
        if (this.answerMustCite == null) {
            this.answerMustCite = false;
        }
    }

    @PreUpdate
    public void preUpdate() {
        this.updatedAt = LocalDateTime.now();
    }
}
