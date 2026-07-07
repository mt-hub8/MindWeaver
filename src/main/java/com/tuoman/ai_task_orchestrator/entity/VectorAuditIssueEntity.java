package com.tuoman.ai_task_orchestrator.entity;

import com.tuoman.ai_task_orchestrator.enums.VectorAuditIssueSeverity;
import com.tuoman.ai_task_orchestrator.enums.VectorAuditIssueType;
import jakarta.persistence.*;
import lombok.Getter;
import lombok.Setter;

import java.time.LocalDateTime;

@Getter
@Setter
@Entity
@Table(name = "vector_audit_issue")
public class VectorAuditIssueEntity {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @Column(name = "audit_run_id", nullable = false)
    private Long auditRunId;

    @Enumerated(EnumType.STRING)
    @Column(name = "issue_type", nullable = false, length = 64)
    private VectorAuditIssueType issueType;

    @Enumerated(EnumType.STRING)
    @Column(name = "severity", nullable = false, length = 32)
    private VectorAuditIssueSeverity severity;

    @Column(name = "collection_id")
    private Long collectionId;

    @Column(name = "document_id")
    private Long documentId;

    @Column(name = "chunk_id")
    private Long chunkId;

    @Column(name = "vector_id", length = 64)
    private String vectorId;

    @Column(name = "stable_vector_key", length = 64)
    private String stableVectorKey;

    @Column(name = "message", nullable = false, columnDefinition = "TEXT")
    private String message;

    @Column(name = "metadata_json", columnDefinition = "TEXT")
    private String metadataJson;

    @Column(name = "created_at", nullable = false)
    private LocalDateTime createdAt;

    @PrePersist
    public void prePersist() {
        if (createdAt == null) {
            createdAt = LocalDateTime.now();
        }
    }
}
