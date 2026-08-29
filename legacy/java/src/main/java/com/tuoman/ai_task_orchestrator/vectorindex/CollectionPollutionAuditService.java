package com.tuoman.ai_task_orchestrator.vectorindex;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.entity.KnowledgeCollectionEntity;
import com.tuoman.ai_task_orchestrator.entity.VectorAuditRunEntity;
import com.tuoman.ai_task_orchestrator.enums.VectorAuditScopeType;
import com.tuoman.ai_task_orchestrator.repository.KnowledgeCollectionRepository;
import lombok.Builder;
import lombok.Getter;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;

import java.util.ArrayList;
import java.util.List;

/**
 * Collection 级向量污染审计服务。
 *
 * 它复用 VectorConsistencyAuditService 的只读 audit 结果，生成面向单个知识库的污染摘要：
 * CrossCollectionVectorLeakRate、Orphan、Missing、Wrong Generation、Purged Residue 等。
 *
 * 关键不变量：这里只做诊断和建议，不执行 cleanup；删除向量必须走带 scope 的 VectorCleanupService。
 */
@Service
@RequiredArgsConstructor
public class CollectionPollutionAuditService {

    private final VectorConsistencyAuditService vectorConsistencyAuditService;

    private final KnowledgeCollectionRepository knowledgeCollectionRepository;

    public CollectionPollutionAuditReport auditCollection(Long collectionId) {
        // collection health 关注向量索引是否污染指定知识库；
        // 它不同于 Knowledge Health，后者评估最终检索/生成质量。
        if (collectionId == null) {
            throw BusinessException.invalidRequest("collectionId 不能为空");
        }
        KnowledgeCollectionEntity collection = knowledgeCollectionRepository.findById(collectionId)
                .orElseThrow(BusinessException::collectionNotFound);

        VectorAuditRunEntity run = vectorConsistencyAuditService.runAudit(
                VectorAuditScopeType.COLLECTION,
                collectionId,
                null
        );
        var issues = vectorConsistencyAuditService.findIssues(run.getId());
        VectorConsistencyAuditService.AuditSummary summary = computeSummary(
                vectorConsistencyAuditService,
                collectionId,
                issues
        );
        List<String> suggestions = buildSuggestions(summary);

        String status = "HEALTHY";
        if (summary.getCrossCollectionVectorLeakRate() > 0 || summary.getPurgedVectorResidueRate() > 0) {
            status = "CRITICAL";
        } else if (summary.getVectorMissingRate() > 0 || summary.getWrongGenerationVectorRate() > 0
                || summary.getVectorOrphanRate() > 0) {
            status = "WARNING";
        }

        return CollectionPollutionAuditReport.builder()
                .collectionId(collectionId)
                .collectionName(collection.getName())
                .auditRunId(run.getId())
                .activeGeneration(null)
                .totalVectors(summary.getTotalVectors())
                .pollutedVectorCount((int) Math.round(summary.getCrossCollectionVectorLeakRate() * summary.getTotalVectors()))
                .crossCollectionVectorLeakRate(summary.getCrossCollectionVectorLeakRate())
                .orphanVectorCount((int) Math.round(summary.getVectorOrphanRate() * summary.getTotalVectors()))
                .missingVectorCount((int) Math.round(summary.getVectorMissingRate() * summary.getTotalVectors()))
                .wrongGenerationVectorCount((int) Math.round(summary.getWrongGenerationVectorRate() * summary.getTotalVectors()))
                .purgedResidueCount((int) Math.round(summary.getPurgedVectorResidueRate() * summary.getTotalVectors()))
                .vectorDuplicateRate(summary.getVectorDuplicateRate())
                .status(status)
                .suggestions(suggestions)
                .build();
    }

    private VectorConsistencyAuditService.AuditSummary computeSummary(
            VectorConsistencyAuditService auditService,
            Long collectionId,
            List<com.tuoman.ai_task_orchestrator.entity.VectorAuditIssueEntity> issues
    ) {
        List<com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreDocument> vectors =
                auditService.scanVectorsForCollection(collectionId);
        int total = vectors.size();
        return VectorConsistencyAuditService.AuditSummary.builder()
                .totalVectors(total)
                .vectorDuplicateRate(rate(countType(issues, com.tuoman.ai_task_orchestrator.enums.VectorAuditIssueType.DUPLICATE_VECTOR), total))
                .vectorOrphanRate(rate(countType(issues, com.tuoman.ai_task_orchestrator.enums.VectorAuditIssueType.ORPHAN_VECTOR), total))
                .vectorMissingRate(rate(countType(issues, com.tuoman.ai_task_orchestrator.enums.VectorAuditIssueType.MISSING_VECTOR), Math.max(1, total)))
                .crossCollectionVectorLeakRate(rate(countType(issues, com.tuoman.ai_task_orchestrator.enums.VectorAuditIssueType.CROSS_COLLECTION_VECTOR_LEAK), total))
                .wrongGenerationVectorRate(rate(countType(issues, com.tuoman.ai_task_orchestrator.enums.VectorAuditIssueType.WRONG_GENERATION_VECTOR), total))
                .modelDimensionMismatchRate(rate(countType(issues, com.tuoman.ai_task_orchestrator.enums.VectorAuditIssueType.MODEL_DIMENSION_MISMATCH), total))
                .trashedVectorVisibleRate(rate(countType(issues, com.tuoman.ai_task_orchestrator.enums.VectorAuditIssueType.TRASHED_VECTOR_VISIBLE), total))
                .purgedVectorResidueRate(rate(countType(issues, com.tuoman.ai_task_orchestrator.enums.VectorAuditIssueType.PURGED_VECTOR_RESIDUE), total))
                .issueCount(issues.size())
                .build();
    }

    private int countType(List<com.tuoman.ai_task_orchestrator.entity.VectorAuditIssueEntity> issues,
                          com.tuoman.ai_task_orchestrator.enums.VectorAuditIssueType type) {
        return (int) issues.stream().filter(issue -> issue.getIssueType() == type).count();
    }

    private double rate(int count, int total) {
        if (total <= 0) {
            return 0.0;
        }
        return (double) count / total;
    }

    private List<String> buildSuggestions(VectorConsistencyAuditService.AuditSummary summary) {
        List<String> suggestions = new ArrayList<>();
        if (summary.getCrossCollectionVectorLeakRate() > 0) {
            suggestions.add("发现跨 collection 向量污染，建议执行 cleanup-pollution");
        }
        if (summary.getVectorMissingRate() > 0) {
            suggestions.add("发现缺失向量，建议重新索引该 collection");
        }
        if (summary.getVectorOrphanRate() > 0) {
            suggestions.add("发现孤儿向量，建议清理孤儿向量");
        }
        if (summary.getWrongGenerationVectorRate() > 0) {
            suggestions.add("发现错误代际向量，建议清理 retired generation");
        }
        if (summary.getPurgedVectorResidueRate() > 0) {
            suggestions.add("发现永久删除残留向量，建议执行 purge cleanup");
        }
        if (suggestions.isEmpty()) {
            suggestions.add("向量索引健康，无需立即处理");
        }
        return suggestions;
    }

    @Getter
    @Builder
    public static class CollectionPollutionAuditReport {
        private final Long collectionId;
        private final String collectionName;
        private final Long auditRunId;
        private final Long activeGeneration;
        private final int totalVectors;
        private final int pollutedVectorCount;
        private final double crossCollectionVectorLeakRate;
        private final int orphanVectorCount;
        private final int missingVectorCount;
        private final int wrongGenerationVectorCount;
        private final int purgedResidueCount;
        private final double vectorDuplicateRate;
        private final String status;
        private final List<String> suggestions;
    }
}
