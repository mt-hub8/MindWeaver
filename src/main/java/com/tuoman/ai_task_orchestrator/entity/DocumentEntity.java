package com.tuoman.ai_task_orchestrator.entity;

import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentPurgeStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import jakarta.persistence.*;
import lombok.Getter;
import lombok.Setter;

import java.time.LocalDateTime;

@Getter
@Setter
@Entity
@Table(name = "document")
public class DocumentEntity {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @Column(name = "original_filename", nullable = false, length = 255)
    private String originalFilename;

    @Column(name = "content_type", length = 100)
    private String contentType;

    @Column(name = "file_size")
    private Long fileSize;

    @Enumerated(EnumType.STRING)
    @Column(nullable = false, length = 50)
    private DocumentStatus status;

    @Enumerated(EnumType.STRING)
    @Column(name = "lifecycle_status", nullable = false, length = 50)
    private DocumentLifecycleStatus lifecycleStatus;

    @Column(name = "deleted_at")
    private LocalDateTime deletedAt;

    @Column(name = "trashed_at")
    private LocalDateTime trashedAt;

    @Column(name = "purge_after")
    private LocalDateTime purgeAfter;

    @Column(name = "purged_at")
    private LocalDateTime purgedAt;

    @Enumerated(EnumType.STRING)
    @Column(name = "purge_status", nullable = false, length = 50)
    private DocumentPurgeStatus purgeStatus;

    @Column(name = "chunk_count", nullable = false)
    private Integer chunkCount;

    @Column(name = "error_message", columnDefinition = "TEXT")
    private String errorMessage;

    @Column(name = "source_text", columnDefinition = "MEDIUMTEXT")
    private String sourceText;

    @Column(name = "current_generation", nullable = false)
    private Integer currentGeneration;

    @Column(name = "reindex_count", nullable = false)
    private Integer reindexCount;

    @Column(name = "file_hash", length = 64)
    private String fileHash;

    @Column(name = "text_hash", length = 64)
    private String textHash;

    @Column(name = "last_reindexed_at")
    private LocalDateTime lastReindexedAt;

    @Column(name = "created_at", nullable = false)
    private LocalDateTime createdAt;

    @Column(name = "updated_at", nullable = false)
    private LocalDateTime updatedAt;

    @PrePersist
    public void prePersist() {
        LocalDateTime now = LocalDateTime.now();
        this.createdAt = now;
        this.updatedAt = now;
        if (this.lifecycleStatus == null) {
            this.lifecycleStatus = DocumentLifecycleStatus.ACTIVE;
        }
        if (this.currentGeneration == null) {
            this.currentGeneration = 1;
        }
        if (this.reindexCount == null) {
            this.reindexCount = 0;
        }
        if (this.purgeStatus == null) {
            this.purgeStatus = DocumentPurgeStatus.NONE;
        }
    }

    @PreUpdate
    public void preUpdate() {
        this.updatedAt = LocalDateTime.now();
    }
}
