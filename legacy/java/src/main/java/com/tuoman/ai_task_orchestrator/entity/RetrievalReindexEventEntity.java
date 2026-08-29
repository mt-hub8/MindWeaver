package com.tuoman.ai_task_orchestrator.entity;

import jakarta.persistence.*;
import lombok.Getter;
import lombok.Setter;

import java.time.LocalDateTime;

@Getter
@Setter
@Entity
@Table(name = "retrieval_reindex_event")
public class RetrievalReindexEventEntity {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @Column(name = "scope_type", nullable = false, length = 32)
    private String scopeType;

    @Column(name = "scope_id")
    private Long scopeId;

    @Column(name = "document_id")
    private Long documentId;

    @Column(name = "collection_id")
    private Long collectionId;

    @Column(name = "chunking_strategy", length = 64)
    private String chunkingStrategy;

    @Column(nullable = false, length = 32)
    private String status;

    @Column(columnDefinition = "TEXT")
    private String message;

    @Column(name = "created_at", nullable = false)
    private LocalDateTime createdAt;

    @PrePersist
    public void prePersist() {
        this.createdAt = LocalDateTime.now();
    }
}
