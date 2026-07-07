package com.tuoman.ai_task_orchestrator.vectorindex;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.entity.VectorAuditIssueEntity;
import com.tuoman.ai_task_orchestrator.entity.VectorAuditRunEntity;
import com.tuoman.ai_task_orchestrator.enums.ChunkStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.VectorAuditIssueSeverity;
import com.tuoman.ai_task_orchestrator.enums.VectorAuditIssueType;
import com.tuoman.ai_task_orchestrator.enums.VectorAuditRunStatus;
import com.tuoman.ai_task_orchestrator.enums.VectorAuditScopeType;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkEmbeddingRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentCollectionRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.repository.VectorAuditIssueRepository;
import com.tuoman.ai_task_orchestrator.repository.VectorAuditRunRepository;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorScanFilter;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStore;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreDocument;
import lombok.Builder;
import lombok.Getter;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.LocalDateTime;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;
import java.util.Set;
import java.util.stream.Collectors;

@Service
@RequiredArgsConstructor
public class VectorConsistencyAuditService {

    private final VectorAuditRunRepository auditRunRepository;

    private final VectorAuditIssueRepository auditIssueRepository;

    private final VectorStore vectorStore;

    private final DocumentRepository documentRepository;

    private final DocumentChunkRepository documentChunkRepository;

    private final DocumentChunkEmbeddingRepository documentChunkEmbeddingRepository;

    private final DocumentCollectionRepository documentCollectionRepository;

    private final VectorGenerationService vectorGenerationService;

    private final ObjectMapper objectMapper;

    @Transactional
    public VectorAuditRunEntity runAudit(VectorAuditScopeType scopeType, Long collectionId, Long documentId) {
        VectorAuditRunEntity run = new VectorAuditRunEntity();
        run.setScopeType(scopeType);
        run.setCollectionId(collectionId);
        run.setDocumentId(documentId);
        run.setStatus(VectorAuditRunStatus.RUNNING);
        run.setStartedAt(LocalDateTime.now());
        run = auditRunRepository.save(run);

        List<VectorAuditIssueEntity> issues = new ArrayList<>();
        List<VectorStoreDocument> vectors = loadVectors(scopeType, collectionId, documentId);
        Optional<Long> activeGeneration = documentId != null
                ? vectorGenerationService.getActiveGeneration(documentId)
                : Optional.empty();

        issues.addAll(findDuplicateIssues(run.getId(), vectors, collectionId));
        issues.addAll(findOrphanIssues(run.getId(), vectors, collectionId));
        issues.addAll(findMissingIssues(run.getId(), scopeType, collectionId, documentId, vectors, activeGeneration.orElse(null)));
        issues.addAll(findCrossCollectionIssues(run.getId(), vectors, collectionId));
        issues.addAll(findWrongGenerationIssues(run.getId(), vectors, collectionId, activeGeneration.orElse(null)));
        issues.addAll(findDimensionMismatchIssues(run.getId(), vectors, collectionId));
        issues.addAll(findTrashedVisibleIssues(run.getId(), vectors, collectionId));
        issues.addAll(findPurgedResidueIssues(run.getId(), vectors, collectionId));

        auditIssueRepository.saveAll(issues);

        AuditSummary summary = buildSummary(vectors, issues);
        try {
            run.setSummaryJson(objectMapper.writeValueAsString(summary));
        } catch (Exception exception) {
            run.setSummaryJson("{}");
        }
        run.setStatus(VectorAuditRunStatus.COMPLETED);
        run.setCompletedAt(LocalDateTime.now());
        return auditRunRepository.save(run);
    }

    public Optional<VectorAuditRunEntity> findRun(Long auditRunId) {
        return auditRunRepository.findById(auditRunId);
    }

    public List<VectorAuditIssueEntity> findIssues(Long auditRunId) {
        return auditIssueRepository.findByAuditRunIdOrderByIdAsc(auditRunId);
    }

    public List<VectorAuditRunEntity> listRuns() {
        return auditRunRepository.findAllByOrderByCreatedAtDesc();
    }

    public List<VectorStoreDocument> scanVectorsForCollection(Long collectionId) {
        return vectorStore.scanByFilter(VectorScanFilter.builder().collectionId(collectionId).build());
    }

    private List<VectorStoreDocument> loadVectors(VectorAuditScopeType scopeType, Long collectionId, Long documentId) {
        if (scopeType == VectorAuditScopeType.DOCUMENT && documentId != null) {
            return vectorStore.scanByFilter(VectorScanFilter.builder().documentId(documentId).build());
        }
        if (collectionId != null) {
            return vectorStore.scanByFilter(VectorScanFilter.builder().collectionId(collectionId).build());
        }
        return documentChunkEmbeddingRepository.findAll().stream()
                .map(entity -> new VectorStoreDocument(
                        entity.getDocumentChunkId(),
                        entity.getDocumentId(),
                        null,
                        List.of(),
                        entity.getEmbeddingProvider(),
                        entity.getEmbeddingModel(),
                        entity.getVectorDimension(),
                        entity.getDistanceMetric(),
                        Map.of(),
                        entity.getVectorId(),
                        entity.getStableVectorKey(),
                        entity.getCollectionId(),
                        entity.getChunkUid(),
                        entity.getVectorGeneration()
                ))
                .toList();
    }

    private List<VectorAuditIssueEntity> findDuplicateIssues(Long runId, List<VectorStoreDocument> vectors, Long collectionId) {
        Map<String, List<VectorStoreDocument>> grouped = vectors.stream()
                .filter(vector -> vector.stableVectorKey() != null)
                .collect(Collectors.groupingBy(VectorStoreDocument::stableVectorKey));
        List<VectorAuditIssueEntity> issues = new ArrayList<>();
        for (Map.Entry<String, List<VectorStoreDocument>> entry : grouped.entrySet()) {
            if (entry.getValue().size() <= 1) {
                continue;
            }
            issues.add(issue(runId, VectorAuditIssueType.DUPLICATE_VECTOR, VectorAuditIssueSeverity.CRITICAL,
                    collectionId, null, null, entry.getValue().get(0).vectorId(), entry.getKey(),
                    "同一 stableVectorKey 存在多个向量，可能导致重复召回"));
        }
        return issues;
    }

    private List<VectorAuditIssueEntity> findOrphanIssues(Long runId, List<VectorStoreDocument> vectors, Long collectionId) {
        Set<Long> chunkIds = documentChunkRepository.findAll().stream().map(DocumentChunkEntity::getId).collect(Collectors.toSet());
        List<VectorAuditIssueEntity> issues = new ArrayList<>();
        for (VectorStoreDocument vector : vectors) {
            if (vector.chunkId() != null && !chunkIds.contains(vector.chunkId())) {
                issues.add(issue(runId, VectorAuditIssueType.ORPHAN_VECTOR, VectorAuditIssueSeverity.WARNING,
                        collectionId, vector.documentId(), vector.chunkId(), vector.vectorId(), vector.stableVectorKey(),
                        "向量存在但对应 chunk 已不存在"));
            }
        }
        return issues;
    }

    private List<VectorAuditIssueEntity> findMissingIssues(
            Long runId,
            VectorAuditScopeType scopeType,
            Long collectionId,
            Long documentId,
            List<VectorStoreDocument> vectors,
            Long activeGeneration
    ) {
        Set<String> vectorKeys = vectors.stream()
                .map(VectorStoreDocument::stableVectorKey)
                .filter(Objects::nonNull)
                .collect(Collectors.toSet());
        List<VectorAuditIssueEntity> issues = new ArrayList<>();
        List<DocumentChunkEntity> chunks = loadActiveChunks(scopeType, collectionId, documentId, activeGeneration);
        for (DocumentChunkEntity chunk : chunks) {
            String uid = chunk.getChunkUid() != null ? chunk.getChunkUid() : chunk.getDocumentId() + "#" + chunk.getChunkIndex();
            boolean found = vectors.stream().anyMatch(vector -> Objects.equals(vector.chunkId(), chunk.getId()));
            if (!found) {
                issues.add(issue(runId, VectorAuditIssueType.MISSING_VECTOR, VectorAuditIssueSeverity.WARNING,
                        collectionId, chunk.getDocumentId(), chunk.getId(), null, uid,
                        "ACTIVE chunk 缺少对应向量，建议重新索引"));
            }
        }
        return issues;
    }

    private List<VectorAuditIssueEntity> findCrossCollectionIssues(Long runId, List<VectorStoreDocument> vectors, Long collectionId) {
        if (collectionId == null) {
            return List.of();
        }
        List<VectorAuditIssueEntity> issues = new ArrayList<>();
        for (VectorStoreDocument vector : vectors) {
            if (vector.collectionId() != null && !Objects.equals(vector.collectionId(), collectionId)) {
                issues.add(issue(runId, VectorAuditIssueType.CROSS_COLLECTION_VECTOR_LEAK, VectorAuditIssueSeverity.CRITICAL,
                        collectionId, vector.documentId(), vector.chunkId(), vector.vectorId(), vector.stableVectorKey(),
                        "向量 payload.collection_id 不属于目标 collection"));
            }
        }
        return issues;
    }

    private List<VectorAuditIssueEntity> findWrongGenerationIssues(
            Long runId,
            List<VectorStoreDocument> vectors,
            Long collectionId,
            Long activeGeneration
    ) {
        if (activeGeneration == null) {
            return List.of();
        }
        List<VectorAuditIssueEntity> issues = new ArrayList<>();
        for (VectorStoreDocument vector : vectors) {
            if (vector.vectorGeneration() != null && !Objects.equals(vector.vectorGeneration(), activeGeneration)) {
                issues.add(issue(runId, VectorAuditIssueType.WRONG_GENERATION_VECTOR, VectorAuditIssueSeverity.WARNING,
                        collectionId, vector.documentId(), vector.chunkId(), vector.vectorId(), vector.stableVectorKey(),
                        "向量 generation 与 active generation 不一致，可能是旧索引残留"));
            }
        }
        return issues;
    }

    private List<VectorAuditIssueEntity> findDimensionMismatchIssues(Long runId, List<VectorStoreDocument> vectors, Long collectionId) {
        List<VectorAuditIssueEntity> issues = new ArrayList<>();
        for (VectorStoreDocument vector : vectors) {
            if (vector.embedding() != null && vector.dimension() != null
                    && vector.embedding().size() != vector.dimension()) {
                issues.add(issue(runId, VectorAuditIssueType.MODEL_DIMENSION_MISMATCH, VectorAuditIssueSeverity.CRITICAL,
                        collectionId, vector.documentId(), vector.chunkId(), vector.vectorId(), vector.stableVectorKey(),
                        "向量实际长度与 embedding_dimension 不一致"));
            }
        }
        return issues;
    }

    private List<VectorAuditIssueEntity> findTrashedVisibleIssues(Long runId, List<VectorStoreDocument> vectors, Long collectionId) {
        List<VectorAuditIssueEntity> issues = new ArrayList<>();
        for (VectorStoreDocument vector : vectors) {
            DocumentEntity document = vector.documentId() == null
                    ? null
                    : documentRepository.findById(vector.documentId()).orElse(null);
            if (document != null && document.getLifecycleStatus() == DocumentLifecycleStatus.TRASHED) {
                String status = vector.metadata() == null ? null : vector.metadata().get("status");
                if (!"TRASHED".equals(status)) {
                    issues.add(issue(runId, VectorAuditIssueType.TRASHED_VECTOR_VISIBLE, VectorAuditIssueSeverity.WARNING,
                            collectionId, vector.documentId(), vector.chunkId(), vector.vectorId(), vector.stableVectorKey(),
                            "垃圾箱文档对应向量仍可能被默认检索召回"));
                }
            }
        }
        return issues;
    }

    private List<VectorAuditIssueEntity> findPurgedResidueIssues(Long runId, List<VectorStoreDocument> vectors, Long collectionId) {
        List<VectorAuditIssueEntity> issues = new ArrayList<>();
        for (VectorStoreDocument vector : vectors) {
            DocumentEntity document = vector.documentId() == null
                    ? null
                    : documentRepository.findById(vector.documentId()).orElse(null);
            if (document != null && document.getLifecycleStatus() == DocumentLifecycleStatus.PURGED) {
                issues.add(issue(runId, VectorAuditIssueType.PURGED_VECTOR_RESIDUE, VectorAuditIssueSeverity.CRITICAL,
                        collectionId, vector.documentId(), vector.chunkId(), vector.vectorId(), vector.stableVectorKey(),
                        "已永久删除文档仍残留向量"));
            }
        }
        return issues;
    }

    private List<DocumentChunkEntity> loadActiveChunks(
            VectorAuditScopeType scopeType,
            Long collectionId,
            Long documentId,
            Long activeGeneration
    ) {
        if (scopeType == VectorAuditScopeType.DOCUMENT && documentId != null) {
            DocumentEntity document = documentRepository.findById(documentId).orElse(null);
            int generation = activeGeneration == null
                    ? (document == null || document.getCurrentGeneration() == null ? 1 : document.getCurrentGeneration())
                    : activeGeneration.intValue();
            return documentChunkRepository.findByDocumentIdAndChunkStatusAndGenerationOrderByChunkIndexAsc(
                    documentId, ChunkStatus.ACTIVE, generation);
        }
        if (collectionId != null) {
            List<Long> documentIds = documentCollectionRepository.findDocumentIdsByCollectionId(collectionId);
            List<DocumentChunkEntity> chunks = new ArrayList<>();
            for (Long id : documentIds) {
                DocumentEntity document = documentRepository.findById(id).orElse(null);
                if (document == null || document.getLifecycleStatus() != DocumentLifecycleStatus.ACTIVE) {
                    continue;
                }
                int generation = document.getCurrentGeneration() == null ? 1 : document.getCurrentGeneration();
                chunks.addAll(documentChunkRepository.findByDocumentIdAndChunkStatusAndGenerationOrderByChunkIndexAsc(
                        id, ChunkStatus.ACTIVE, generation));
            }
            return chunks;
        }
        return documentChunkRepository.findAll().stream()
                .filter(chunk -> chunk.getChunkStatus() == ChunkStatus.ACTIVE)
                .toList();
    }

    private VectorAuditIssueEntity issue(
            Long runId,
            VectorAuditIssueType type,
            VectorAuditIssueSeverity severity,
            Long collectionId,
            Long documentId,
            Long chunkId,
            String vectorId,
            String stableVectorKey,
            String message
    ) {
        VectorAuditIssueEntity issue = new VectorAuditIssueEntity();
        issue.setAuditRunId(runId);
        issue.setIssueType(type);
        issue.setSeverity(severity);
        issue.setCollectionId(collectionId);
        issue.setDocumentId(documentId);
        issue.setChunkId(chunkId);
        issue.setVectorId(vectorId);
        issue.setStableVectorKey(stableVectorKey);
        issue.setMessage(message);
        return issue;
    }

    private AuditSummary buildSummary(List<VectorStoreDocument> vectors, List<VectorAuditIssueEntity> issues) {
        int totalVectors = vectors.size();
        int duplicate = countType(issues, VectorAuditIssueType.DUPLICATE_VECTOR);
        int orphan = countType(issues, VectorAuditIssueType.ORPHAN_VECTOR);
        int missing = countType(issues, VectorAuditIssueType.MISSING_VECTOR);
        int cross = countType(issues, VectorAuditIssueType.CROSS_COLLECTION_VECTOR_LEAK);
        int wrongGen = countType(issues, VectorAuditIssueType.WRONG_GENERATION_VECTOR);
        int dimension = countType(issues, VectorAuditIssueType.MODEL_DIMENSION_MISMATCH);
        int trashed = countType(issues, VectorAuditIssueType.TRASHED_VECTOR_VISIBLE);
        int purged = countType(issues, VectorAuditIssueType.PURGED_VECTOR_RESIDUE);

        return AuditSummary.builder()
                .totalVectors(totalVectors)
                .vectorDuplicateRate(rate(duplicate, totalVectors))
                .vectorOrphanRate(rate(orphan, totalVectors))
                .vectorMissingRate(rate(missing, Math.max(1, totalVectors + missing)))
                .crossCollectionVectorLeakRate(rate(cross, totalVectors))
                .wrongGenerationVectorRate(rate(wrongGen, totalVectors))
                .modelDimensionMismatchRate(rate(dimension, totalVectors))
                .trashedVectorVisibleRate(rate(trashed, totalVectors))
                .purgedVectorResidueRate(rate(purged, totalVectors))
                .issueCount(issues.size())
                .build();
    }

    private int countType(List<VectorAuditIssueEntity> issues, VectorAuditIssueType type) {
        return (int) issues.stream().filter(issue -> issue.getIssueType() == type).count();
    }

    private double rate(int count, int total) {
        if (total <= 0) {
            return 0.0;
        }
        return (double) count / total;
    }

    @Getter
    @Builder
    public static class AuditSummary {
        private final int totalVectors;
        private final double vectorDuplicateRate;
        private final double vectorOrphanRate;
        private final double vectorMissingRate;
        private final double crossCollectionVectorLeakRate;
        private final double wrongGenerationVectorRate;
        private final double modelDimensionMismatchRate;
        private final double trashedVectorVisibleRate;
        private final double purgedVectorResidueRate;
        private final int issueCount;
    }
}
