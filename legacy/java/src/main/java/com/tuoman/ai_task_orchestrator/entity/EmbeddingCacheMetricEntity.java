package com.tuoman.ai_task_orchestrator.entity;

import jakarta.persistence.*;
import lombok.Getter;
import lombok.Setter;

import java.time.LocalDateTime;

@Getter
@Setter
@Entity
@Table(
        name = "embedding_cache_metric",
        uniqueConstraints = @UniqueConstraint(
                name = "uk_embedding_cache_metric_space",
                columnNames = {"provider", "model", "dimension"}
        )
)
public class EmbeddingCacheMetricEntity {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @Column(nullable = false, length = 100)
    private String provider;

    @Column(nullable = false, length = 255)
    private String model;

    @Column(nullable = false)
    private Integer dimension;

    @Column(name = "hit_count", nullable = false)
    private Long hitCount = 0L;

    @Column(name = "miss_count", nullable = false)
    private Long missCount = 0L;

    @Column(name = "write_count", nullable = false)
    private Long writeCount = 0L;

    @Column(name = "conflict_count", nullable = false)
    private Long conflictCount = 0L;

    @Column(name = "provider_call_count", nullable = false)
    private Long providerCallCount = 0L;

    @Column(name = "saved_provider_call_count", nullable = false)
    private Long savedProviderCallCount = 0L;

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
