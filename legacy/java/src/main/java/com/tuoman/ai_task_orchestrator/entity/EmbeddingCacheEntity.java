package com.tuoman.ai_task_orchestrator.entity;

import jakarta.persistence.*;
import lombok.Getter;
import lombok.Setter;

import java.time.LocalDateTime;

@Getter
@Setter
@Entity
@Table(
        name = "embedding_cache",
        uniqueConstraints = @UniqueConstraint(
                name = "uk_embedding_cache_key",
                columnNames = {"chunk_hash", "provider", "model", "dimension"}
        )
)
public class EmbeddingCacheEntity {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @Column(name = "chunk_hash", nullable = false, length = 64)
    private String chunkHash;

    @Column(nullable = false, length = 100)
    private String provider;

    @Column(nullable = false, length = 255)
    private String model;

    @Column(nullable = false)
    private Integer dimension;

    @Column(name = "embedding_json", nullable = false, columnDefinition = "LONGTEXT")
    private String embeddingJson;

    @Column(name = "created_at", nullable = false)
    private LocalDateTime createdAt;

    @Column(name = "updated_at", nullable = false)
    private LocalDateTime updatedAt;

    @PrePersist
    public void prePersist() {
        LocalDateTime now = LocalDateTime.now();
        this.createdAt = now;
        this.updatedAt = now;
    }

    @PreUpdate
    public void preUpdate() {
        this.updatedAt = LocalDateTime.now();
    }
}
