package com.tuoman.ai_task_orchestrator.entity;

import com.tuoman.ai_task_orchestrator.enums.AgentTaskStatus;
import jakarta.persistence.*;
import lombok.Getter;
import lombok.Setter;

import java.time.LocalDateTime;

@Getter
@Setter
@Entity
@Table(name = "agent_task")
public class AgentTaskEntity {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @Column(nullable = false, length = 256)
    private String title;

    @Column(nullable = false, columnDefinition = "TEXT")
    private String objective;

    @Column(name = "collection_id")
    private Long collectionId;

    @Column(name = "collection_name", length = 128)
    private String collectionName;

    @Enumerated(EnumType.STRING)
    @Column(nullable = false, length = 50)
    private AgentTaskStatus status;

    @Column(columnDefinition = "TEXT")
    private String result;

    @Column(name = "error_code", length = 100)
    private String errorCode;

    @Column(name = "error_message", columnDefinition = "TEXT")
    private String errorMessage;

    @Column(name = "trace_id", length = 100)
    private String traceId;

    @Column(name = "llm_provider", length = 100)
    private String llmProvider;

    @Column(name = "llm_model", length = 128)
    private String llmModel;

    @Column(name = "embedding_provider", length = 100)
    private String embeddingProvider;

    @Column(name = "embedding_model", length = 128)
    private String embeddingModel;

    @Column(name = "input_tokens")
    private Integer inputTokens;

    @Column(name = "output_tokens")
    private Integer outputTokens;

    @Column(name = "retrieval_count")
    private Integer retrievalCount;

    @Column(name = "citation_count")
    private Integer citationCount;

    @Column(name = "step_count")
    private Integer stepCount;

    @Column(name = "tool_execution_count")
    private Integer toolExecutionCount;

    @Column(name = "failed_step_count")
    private Integer failedStepCount;

    @Column(name = "final_report_latency_ms")
    private Long finalReportLatencyMs;

    @Column(name = "created_at", nullable = false)
    private LocalDateTime createdAt;

    @Column(name = "updated_at", nullable = false)
    private LocalDateTime updatedAt;

    @Column(name = "started_at")
    private LocalDateTime startedAt;

    @Column(name = "completed_at")
    private LocalDateTime completedAt;

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
            status = AgentTaskStatus.PENDING;
        }
    }

    @PreUpdate
    void onUpdate() {
        updatedAt = LocalDateTime.now();
    }
}
