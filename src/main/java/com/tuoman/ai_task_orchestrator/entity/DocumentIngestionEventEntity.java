package com.tuoman.ai_task_orchestrator.entity;

import com.tuoman.ai_task_orchestrator.enums.IngestionEventStatus;
import com.tuoman.ai_task_orchestrator.enums.IngestionEventType;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStep;
import jakarta.persistence.*;
import lombok.Getter;
import lombok.Setter;

import java.time.LocalDateTime;

@Getter
@Setter
@Entity
@Table(name = "document_ingestion_event")
public class DocumentIngestionEventEntity {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @Column(name = "task_id", nullable = false)
    private Long taskId;

    @Enumerated(EnumType.STRING)
    @Column(name = "event_type", nullable = false, length = 50)
    private IngestionEventType eventType;

    @Enumerated(EnumType.STRING)
    @Column(length = 50)
    private IngestionTaskStep step;

    @Enumerated(EnumType.STRING)
    @Column(nullable = false, length = 50)
    private IngestionEventStatus status;

    @Column(length = 500)
    private String message;

    @Column(name = "display_message", nullable = false, length = 500)
    private String displayMessage;

    @Column(name = "duration_ms")
    private Long durationMs;

    @Column(name = "metadata_json", columnDefinition = "TEXT")
    private String metadataJson;

    @Column(name = "error_code", length = 100)
    private String errorCode;

    @Column(name = "error_message", columnDefinition = "TEXT")
    private String errorMessage;

    @Column(name = "trace_id", length = 100)
    private String traceId;

    @Column(name = "created_at", nullable = false)
    private LocalDateTime createdAt;

    @PrePersist
    public void prePersist() {
        if (this.createdAt == null) {
            this.createdAt = LocalDateTime.now();
        }
    }
}
