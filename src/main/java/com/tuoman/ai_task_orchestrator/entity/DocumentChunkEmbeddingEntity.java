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

    @Column(name = "created_at", nullable = false)
    private LocalDateTime createdAt;

    @PrePersist
    public void prePersist() {
        this.createdAt = LocalDateTime.now();
    }
}
