package com.tuoman.ai_task_orchestrator.entity;

import jakarta.persistence.*;
import lombok.Getter;
import lombok.Setter;

import java.time.LocalDateTime;

@Getter
@Setter
@Entity
@Table(name = "document_chunk_embedding")
public class DocumentChunkEmbeddingEntity {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @Column(name = "document_id", nullable = false)
    private Long documentId;

    @Column(name = "document_chunk_id", nullable = false)
    private Long documentChunkId;

    @Column(name = "embedding_provider", nullable = false, length = 100)
    private String embeddingProvider;

    @Column(name = "embedding_model", nullable = false, length = 100)
    private String embeddingModel;

    @Column(name = "vector_dimension", nullable = false)
    private Integer vectorDimension;

    @Column(name = "distance_metric", nullable = false, length = 50)
    private String distanceMetric;

    @Column(name = "embedding_vector", nullable = false, columnDefinition = "TEXT")
    private String embeddingVector;

    @Column(name = "vector_id", length = 64)
    private String vectorId;

    @Column(name = "stable_vector_key", length = 64)
    private String stableVectorKey;

    @Column(name = "collection_id")
    private Long collectionId;

    @Column(name = "chunk_uid", length = 128)
    private String chunkUid;

    @Column(name = "vector_generation")
    private Long vectorGeneration;

    @Column(name = "content_hash", length = 64)
    private String contentHash;

    @Column(name = "metadata_hash", length = 64)
    private String metadataHash;

    @Column(name = "payload_json", columnDefinition = "TEXT")
    private String payloadJson;

    @Column(name = "payload_status", length = 32)
    private String payloadStatus;

    @Column(name = "created_at", nullable = false)
    private LocalDateTime createdAt;

    @Column(name = "updated_at")
    private LocalDateTime updatedAt;

    @PrePersist
    public void prePersist() {
        this.createdAt = LocalDateTime.now();
        this.updatedAt = LocalDateTime.now();
    }

    @PreUpdate
    public void preUpdate() {
        this.updatedAt = LocalDateTime.now();
    }
}
