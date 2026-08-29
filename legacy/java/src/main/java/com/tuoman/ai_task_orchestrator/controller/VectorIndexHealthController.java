package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.dto.VectorCleanupResponse;
import com.tuoman.ai_task_orchestrator.dto.VectorIndexAuditIssueResponse;
import com.tuoman.ai_task_orchestrator.dto.VectorIndexAuditRequest;
import com.tuoman.ai_task_orchestrator.dto.VectorIndexAuditRunResponse;
import com.tuoman.ai_task_orchestrator.dto.VectorIndexSummaryResponse;
import com.tuoman.ai_task_orchestrator.entity.VectorAuditIssueEntity;
import com.tuoman.ai_task_orchestrator.entity.VectorAuditRunEntity;
import com.tuoman.ai_task_orchestrator.enums.VectorAuditScopeType;
import com.tuoman.ai_task_orchestrator.vectorindex.CollectionPollutionAuditService;
import com.tuoman.ai_task_orchestrator.vectorindex.VectorCleanupService;
import com.tuoman.ai_task_orchestrator.vectorindex.VectorConsistencyAuditService;
import com.tuoman.ai_task_orchestrator.vectorindex.VectorIndexSummaryService;
import lombok.RequiredArgsConstructor;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.util.List;

@RestController
@RequestMapping("/vector-index")
@RequiredArgsConstructor
/**
 * Vector Index Health HTTP 入口。
 *
 * 暴露向量索引 summary、audit 和 cleanup 能力；
 * audit 是只读诊断，cleanup 是显式带 scope 的破坏性操作。
 */
public class VectorIndexHealthController {

    private final VectorIndexSummaryService summaryService;

    private final VectorConsistencyAuditService auditService;

    private final CollectionPollutionAuditService collectionPollutionAuditService;

    private final VectorCleanupService cleanupService;

    @GetMapping("/summary")
    public VectorIndexSummaryResponse summary() {
        VectorIndexSummaryService.VectorIndexSummary summary = summaryService.getSummary();
        return VectorIndexSummaryResponse.builder()
                .totalVectors(summary.getTotalVectors())
                .totalChunks(summary.getTotalChunks())
                .collectionCount(summary.getCollectionCount())
                .activeGenerationCount(summary.getActiveGenerationCount())
                .build();
    }

    @PostMapping("/audit")
    public VectorIndexAuditRunResponse audit(@RequestBody VectorIndexAuditRequest request) {
        VectorAuditScopeType scopeType = request.getScopeType() == null
                ? VectorAuditScopeType.ALL
                : VectorAuditScopeType.valueOf(request.getScopeType());
        VectorAuditRunEntity run = auditService.runAudit(scopeType, request.getCollectionId(), request.getDocumentId());
        return toRunResponse(run);
    }

    @GetMapping("/audits")
    public List<VectorIndexAuditRunResponse> listAudits() {
        return auditService.listRuns().stream().map(this::toRunResponse).toList();
    }

    @GetMapping("/audits/{auditRunId}")
    public VectorIndexAuditRunResponse getAudit(@PathVariable Long auditRunId) {
        return auditService.findRun(auditRunId).map(this::toRunResponse)
                .orElseThrow(() -> new IllegalArgumentException("audit run not found"));
    }

    @GetMapping("/audits/{auditRunId}/issues")
    public List<VectorIndexAuditIssueResponse> listIssues(@PathVariable Long auditRunId) {
        return auditService.findIssues(auditRunId).stream().map(this::toIssueResponse).toList();
    }

    @PostMapping("/collections/{collectionId}/audit")
    public CollectionPollutionAuditService.CollectionPollutionAuditReport auditCollection(
            @PathVariable Long collectionId
    ) {
        return collectionPollutionAuditService.auditCollection(collectionId);
    }

    @PostMapping("/documents/{documentId}/audit")
    public VectorIndexAuditRunResponse auditDocument(@PathVariable Long documentId) {
        VectorAuditRunEntity run = auditService.runAudit(VectorAuditScopeType.DOCUMENT, null, documentId);
        return toRunResponse(run);
    }

    @PostMapping("/cleanup/orphans")
    public VectorCleanupResponse cleanupOrphans(@RequestBody VectorIndexAuditRequest request) {
        if (request.getCollectionId() == null) {
            throw new IllegalArgumentException("collectionId is required");
        }
        return toCleanupResponse(cleanupService.cleanupOrphanVectors(request.getCollectionId()));
    }

    @PostMapping("/cleanup/retired-generations")
    public VectorCleanupResponse cleanupRetiredGenerations(@RequestBody VectorIndexAuditRequest request) {
        if (request.getCollectionId() == null) {
            throw new IllegalArgumentException("collectionId is required");
        }
        return toCleanupResponse(cleanupService.cleanupRetiredGenerations(request.getCollectionId()));
    }

    @PostMapping("/cleanup/purged-residue")
    public VectorCleanupResponse cleanupPurgedResidue() {
        return toCleanupResponse(cleanupService.cleanupPurgedDocumentResidue());
    }

    @PostMapping("/collections/{collectionId}/cleanup-pollution")
    public VectorCleanupResponse cleanupPollution(@PathVariable Long collectionId) {
        return toCleanupResponse(cleanupService.cleanupPollutedVectors(collectionId));
    }

    private VectorIndexAuditRunResponse toRunResponse(VectorAuditRunEntity run) {
        return VectorIndexAuditRunResponse.builder()
                .id(run.getId())
                .scopeType(run.getScopeType().name())
                .collectionId(run.getCollectionId())
                .documentId(run.getDocumentId())
                .status(run.getStatus().name())
                .startedAt(run.getStartedAt())
                .completedAt(run.getCompletedAt())
                .summaryJson(run.getSummaryJson())
                .build();
    }

    private VectorIndexAuditIssueResponse toIssueResponse(VectorAuditIssueEntity issue) {
        return VectorIndexAuditIssueResponse.builder()
                .id(issue.getId())
                .issueType(issue.getIssueType().name())
                .severity(issue.getSeverity().name())
                .collectionId(issue.getCollectionId())
                .documentId(issue.getDocumentId())
                .chunkId(issue.getChunkId())
                .vectorId(issue.getVectorId())
                .stableVectorKey(issue.getStableVectorKey())
                .message(issue.getMessage())
                .build();
    }

    private VectorCleanupResponse toCleanupResponse(VectorCleanupService.VectorCleanupResult result) {
        return VectorCleanupResponse.builder()
                .requestedCount(result.getRequestedCount())
                .deletedCount(result.getDeletedCount())
                .failedCount(result.getFailedCount())
                .skippedCount(result.getSkippedCount())
                .warnings(result.getWarnings())
                .errors(result.getErrors())
                .build();
    }
}
