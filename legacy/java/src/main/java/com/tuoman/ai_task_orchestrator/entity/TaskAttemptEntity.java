package com.tuoman.ai_task_orchestrator.entity;

import com.tuoman.ai_task_orchestrator.enums.TaskAttemptStatus;
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
import jakarta.persistence.UniqueConstraint;
import lombok.Getter;
import lombok.Setter;

import java.time.LocalDateTime;

@Getter
@Setter
@Entity
@Table(
        name = "task_attempt",
        uniqueConstraints = @UniqueConstraint(name = "uk_task_attempt_task_attempt_no", columnNames = {"task_id", "attempt_no"})
)
public class TaskAttemptEntity {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @Column(name = "task_id", nullable = false)
    private Long taskId;

    @Column(name = "attempt_no", nullable = false)
    private Integer attemptNo;

    @Enumerated(EnumType.STRING)
    @Column(nullable = false, length = 32)
    private TaskAttemptStatus status;

    @Column(name = "worker_id", length = 128)
    private String workerId;

    @Column(name = "llm_provider", length = 64)
    private String llmProvider;

    @Column(name = "llm_model", length = 128)
    private String llmModel;

    @Column(name = "prompt_template_code", length = 128)
    private String promptTemplateCode;

    @Column(name = "rendered_prompt", columnDefinition = "TEXT")
    private String renderedPrompt;

    @Column(name = "prompt_token_count")
    private Integer promptTokenCount;

    @Column(name = "completion_token_count")
    private Integer completionTokenCount;

    @Column(name = "total_token_count")
    private Integer totalTokenCount;

    @Column(name = "llm_latency_ms")
    private Long llmLatencyMs;

    @Column(name = "error_message", columnDefinition = "TEXT")
    private String errorMessage;

    @Column(name = "started_at", nullable = false)
    private LocalDateTime startedAt;

    @Column(name = "finished_at")
    private LocalDateTime finishedAt;

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
