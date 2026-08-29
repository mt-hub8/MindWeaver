package com.tuoman.ai_task_orchestrator.entity;

import com.tuoman.ai_task_orchestrator.enums.MemoryScope;
import com.tuoman.ai_task_orchestrator.enums.MemorySourceType;
import com.tuoman.ai_task_orchestrator.enums.MemoryStatus;
import com.tuoman.ai_task_orchestrator.enums.MemoryType;
import com.tuoman.ai_task_orchestrator.enums.MemoryVisibility;
import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.EnumType;
import jakarta.persistence.Enumerated;
import jakarta.persistence.GeneratedValue;
import jakarta.persistence.GenerationType;
import jakarta.persistence.Id;
import jakarta.persistence.PrePersist;
import jakarta.persistence.PreUpdate;
import jakarta.persistence.Table;
import lombok.Getter;
import lombok.Setter;

import java.math.BigDecimal;
import java.time.LocalDateTime;

@Getter
@Setter
@Entity
@Table(name = "memory_item")
public class MemoryItemEntity {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @Column(name = "memory_key", length = 200)
    private String memoryKey;

    @Column(nullable = false, length = 256)
    private String title;

    @Column(nullable = false, columnDefinition = "TEXT")
    private String content;

    @Enumerated(EnumType.STRING)
    @Column(name = "memory_type", nullable = false, length = 50)
    private MemoryType memoryType;

    @Enumerated(EnumType.STRING)
    @Column(name = "memory_scope", nullable = false, length = 50)
    private MemoryScope memoryScope;

    @Enumerated(EnumType.STRING)
    @Column(nullable = false, length = 32)
    private MemoryVisibility visibility;

    @Enumerated(EnumType.STRING)
    @Column(nullable = false, length = 32)
    private MemoryStatus status;

    @Enumerated(EnumType.STRING)
    @Column(name = "source_type", nullable = false, length = 50)
    private MemorySourceType sourceType;

    @Column(name = "source_id", length = 128)
    private String sourceId;

    @Column(name = "project_id")
    private Long projectId;

    @Column(name = "agent_profile_id")
    private Long agentProfileId;

    @Column(name = "task_id")
    private Long taskId;

    @Column(nullable = false, precision = 5, scale = 4)
    private BigDecimal confidence;

    @Column(nullable = false)
    private Integer importance;

    @Column(name = "expires_at")
    private LocalDateTime expiresAt;

    @Column(name = "last_used_at")
    private LocalDateTime lastUsedAt;

    @Column(name = "use_count", nullable = false)
    private Long useCount;

    @Column(name = "created_at", nullable = false)
    private LocalDateTime createdAt;

    @Column(name = "updated_at", nullable = false)
    private LocalDateTime updatedAt;

    @Column(name = "deleted_at")
    private LocalDateTime deletedAt;

    @Column(name = "metadata_json", columnDefinition = "TEXT")
    private String metadataJson;

    @PrePersist
    void onCreate() {
        LocalDateTime now = LocalDateTime.now();
        if (createdAt == null) {
            createdAt = now;
        }
        if (updatedAt == null) {
            updatedAt = now;
        }
        if (status == null) {
            status = MemoryStatus.ACTIVE;
        }
        if (visibility == null) {
            visibility = memoryScope == MemoryScope.SHARED
                    ? MemoryVisibility.SHARED : MemoryVisibility.PRIVATE;
        }
        if (sourceType == null) {
            sourceType = MemorySourceType.MANUAL;
        }
        if (confidence == null) {
            confidence = BigDecimal.ONE;
        }
        if (importance == null) {
            importance = 50;
        }
        if (useCount == null) {
            useCount = 0L;
        }
    }

    @PreUpdate
    void onUpdate() {
        updatedAt = LocalDateTime.now();
    }
}
