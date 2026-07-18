package com.tuoman.ai_task_orchestrator.entity;

import com.tuoman.ai_task_orchestrator.enums.MemoryScope;
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

import java.time.LocalDateTime;

@Getter
@Setter
@Entity
@Table(name = "agent_profile")
public class AgentProfileEntity {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @Column(name = "agent_key", nullable = false, unique = true, length = 100)
    private String agentKey;

    @Column(name = "display_name", nullable = false, length = 128)
    private String displayName;

    @Column(name = "role_name", nullable = false, length = 128)
    private String roleName;

    @Column(columnDefinition = "TEXT")
    private String description;

    @Column(name = "system_instruction", nullable = false, columnDefinition = "TEXT")
    private String systemInstruction;

    @Enumerated(EnumType.STRING)
    @Column(name = "default_memory_scope", nullable = false, length = 32)
    private MemoryScope defaultMemoryScope;

    @Column(nullable = false)
    private boolean enabled;

    @Column(name = "created_at", nullable = false)
    private LocalDateTime createdAt;

    @Column(name = "updated_at", nullable = false)
    private LocalDateTime updatedAt;

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
        if (defaultMemoryScope == null) {
            defaultMemoryScope = MemoryScope.AGENT;
        }
    }

    @PreUpdate
    void onUpdate() {
        updatedAt = LocalDateTime.now();
    }
}
