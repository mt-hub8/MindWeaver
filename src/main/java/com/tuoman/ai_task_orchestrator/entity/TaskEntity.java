package com.tuoman.ai_task_orchestrator.entity;

import com.tuoman.ai_task_orchestrator.enums.TaskStatus;
import jakarta.persistence.*;
import lombok.Getter;
import lombok.Setter;

import java.time.LocalDateTime;

@Getter
@Setter
@Entity
@Table(name = "task")
public class TaskEntity {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @Column(nullable = false, columnDefinition = "TEXT")
    private String prompt;

    @Enumerated(EnumType.STRING)
    @Column(nullable = false, length = 32)
    private TaskStatus status;

    @Column(name = "error_message", columnDefinition = "TEXT")
    private String errorMessage;

    @Column(name = "retry_count", nullable = false)
    private int retryCount;

    @Column(name = "max_retry", nullable = false)
    private int maxRetry;

    @Column(name = "next_retry_at")
    private LocalDateTime nextRetryAt;

    @Column(name = "timeout_seconds", nullable = false)
    private Integer timeoutSeconds;

    @Column(name = "timeout_at")
    private LocalDateTime timeoutAt;

    @Column(name = "result_content", columnDefinition = "TEXT")
    private String resultContent;

    @Column(name = "llm_model", length = 100)
    private String llmModel;

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
