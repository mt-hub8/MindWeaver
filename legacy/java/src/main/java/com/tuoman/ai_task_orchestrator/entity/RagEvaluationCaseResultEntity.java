package com.tuoman.ai_task_orchestrator.entity;

import com.tuoman.ai_task_orchestrator.kbhealth.RagEvaluationRetrievalStrategy;
import jakarta.persistence.*;
import lombok.Getter;
import lombok.Setter;

import java.time.LocalDateTime;

@Getter
@Setter
@Entity
@Table(name = "rag_evaluation_case_result")
public class RagEvaluationCaseResultEntity {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @Column(name = "run_id", nullable = false)
    private Long runId;

    @Column(name = "case_ref_id", nullable = false)
    private Long caseRefId;

    @Column(name = "case_id", nullable = false, length = 128)
    private String caseId;

    @Column(nullable = false, columnDefinition = "TEXT")
    private String query;

    @Enumerated(EnumType.STRING)
    @Column(nullable = false, length = 64)
    private RagEvaluationRetrievalStrategy strategy;

    @Column(name = "top_k", nullable = false)
    private Integer topK;

    @Column(name = "retrieved_chunks_json", columnDefinition = "MEDIUMTEXT")
    private String retrievedChunksJson;

    @Column(name = "generated_answer", columnDefinition = "TEXT")
    private String generatedAnswer;

    @Column(name = "citations_json", columnDefinition = "TEXT")
    private String citationsJson;

    @Column(name = "context_bundle_json", columnDefinition = "MEDIUMTEXT")
    private String contextBundleJson;

    @Column(name = "citation_verification_json", columnDefinition = "MEDIUMTEXT")
    private String citationVerificationJson;

    @Column(name = "unsupported_claim_report_json", columnDefinition = "MEDIUMTEXT")
    private String unsupportedClaimReportJson;

    @Column(name = "refusal_decision_json", columnDefinition = "MEDIUMTEXT")
    private String refusalDecisionJson;

    @Column(name = "grounding_score_json", columnDefinition = "MEDIUMTEXT")
    private String groundingScoreJson;

    @Column(name = "retrieval_metrics_json", columnDefinition = "MEDIUMTEXT")
    private String retrievalMetricsJson;

    @Column(name = "generation_metrics_json", columnDefinition = "MEDIUMTEXT")
    private String generationMetricsJson;

    @Column(name = "quality_score")
    private Integer qualityScore;

    @Column(name = "diagnosis_json", columnDefinition = "MEDIUMTEXT")
    private String diagnosisJson;

    @Column(name = "latency_ms")
    private Long latencyMs;

    @Column(name = "error_code", length = 64)
    private String errorCode;

    @Column(name = "error_message", columnDefinition = "TEXT")
    private String errorMessage;

    @Column(name = "created_at", nullable = false)
    private LocalDateTime createdAt;

    @PrePersist
    public void prePersist() {
        this.createdAt = LocalDateTime.now();
    }
}
