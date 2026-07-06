package com.tuoman.ai_task_orchestrator.entity;

import com.tuoman.ai_task_orchestrator.enums.DuplicatePolicy;
import com.tuoman.ai_task_orchestrator.enums.UploadBatchStatus;
import jakarta.persistence.*;
import lombok.Getter;
import lombok.Setter;

import java.time.LocalDateTime;

@Getter
@Setter
@Entity
@Table(name = "upload_batch")
public class UploadBatchEntity {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @Column(length = 255)
    private String name;

    @Enumerated(EnumType.STRING)
    @Column(nullable = false, length = 50)
    private UploadBatchStatus status;

    @Enumerated(EnumType.STRING)
    @Column(name = "duplicate_policy", nullable = false, length = 50)
    private DuplicatePolicy duplicatePolicy;

    @Column(name = "collection_id")
    private Long collectionId;

    @Column(name = "total_count", nullable = false)
    private Integer totalCount;

    @Column(name = "pending_count", nullable = false)
    private Integer pendingCount;

    @Column(name = "queued_count", nullable = false)
    private Integer queuedCount;

    @Column(name = "processing_count", nullable = false)
    private Integer processingCount;

    @Column(name = "completed_count", nullable = false)
    private Integer completedCount;

    @Column(name = "failed_count", nullable = false)
    private Integer failedCount;

    @Column(name = "skipped_count", nullable = false)
    private Integer skippedCount;

    @Column(name = "duplicate_count", nullable = false)
    private Integer duplicateCount;

    @Column(name = "canceled_count", nullable = false)
    private Integer canceledCount;

    @Column(name = "summary_message", length = 512)
    private String summaryMessage;

    @Column(name = "created_at", nullable = false)
    private LocalDateTime createdAt;

    @Column(name = "updated_at", nullable = false)
    private LocalDateTime updatedAt;

    @Column(name = "started_at")
    private LocalDateTime startedAt;

    @Column(name = "completed_at")
    private LocalDateTime completedAt;

    @PrePersist
    public void prePersist() {
        LocalDateTime now = LocalDateTime.now();
        this.createdAt = now;
        this.updatedAt = now;
        if (this.status == null) {
            this.status = UploadBatchStatus.CREATED;
        }
        if (this.duplicatePolicy == null) {
            this.duplicatePolicy = DuplicatePolicy.SKIP;
        }
        initCounts();
    }

    @PreUpdate
    public void preUpdate() {
        this.updatedAt = LocalDateTime.now();
    }

    public void initCounts() {
        if (totalCount == null) {
            totalCount = 0;
        }
        if (pendingCount == null) {
            pendingCount = 0;
        }
        if (queuedCount == null) {
            queuedCount = 0;
        }
        if (processingCount == null) {
            processingCount = 0;
        }
        if (completedCount == null) {
            completedCount = 0;
        }
        if (failedCount == null) {
            failedCount = 0;
        }
        if (skippedCount == null) {
            skippedCount = 0;
        }
        if (duplicateCount == null) {
            duplicateCount = 0;
        }
        if (canceledCount == null) {
            canceledCount = 0;
        }
    }
}
