package com.tuoman.ai_task_orchestrator.entity;

import com.tuoman.ai_task_orchestrator.kbhealth.RagEvaluationDatasetStatus;
import com.tuoman.ai_task_orchestrator.kbhealth.RagEvaluationDatasetType;
import jakarta.persistence.*;
import lombok.Getter;
import lombok.Setter;

import java.time.LocalDateTime;

@Getter
@Setter
@Entity
@Table(name = "rag_evaluation_dataset")
public class RagEvaluationDatasetEntity {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @Column(nullable = false, length = 255)
    private String name;

    @Column(columnDefinition = "TEXT")
    private String description;

    @Enumerated(EnumType.STRING)
    @Column(name = "dataset_type", nullable = false, length = 64)
    private RagEvaluationDatasetType datasetType;

    @Enumerated(EnumType.STRING)
    @Column(nullable = false, length = 32)
    private RagEvaluationDatasetStatus status;

    @Column(name = "case_count", nullable = false)
    private Integer caseCount;

    @Column(name = "source_type", length = 64)
    private String sourceType;

    @Column(name = "created_at", nullable = false)
    private LocalDateTime createdAt;

    @Column(name = "updated_at", nullable = false)
    private LocalDateTime updatedAt;

    @PrePersist
    public void prePersist() {
        LocalDateTime now = LocalDateTime.now();
        this.createdAt = now;
        this.updatedAt = now;
        if (this.status == null) {
            this.status = RagEvaluationDatasetStatus.DRAFT;
        }
        if (this.datasetType == null) {
            this.datasetType = RagEvaluationDatasetType.GOLD_TEST_SET;
        }
        if (this.caseCount == null) {
            this.caseCount = 0;
        }
    }

    @PreUpdate
    public void preUpdate() {
        this.updatedAt = LocalDateTime.now();
    }
}
