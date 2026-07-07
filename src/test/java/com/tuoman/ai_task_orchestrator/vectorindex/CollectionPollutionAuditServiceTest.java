package com.tuoman.ai_task_orchestrator.vectorindex;

import com.tuoman.ai_task_orchestrator.entity.KnowledgeCollectionEntity;
import com.tuoman.ai_task_orchestrator.entity.VectorAuditIssueEntity;
import com.tuoman.ai_task_orchestrator.entity.VectorAuditRunEntity;
import com.tuoman.ai_task_orchestrator.enums.CollectionStatus;
import com.tuoman.ai_task_orchestrator.enums.VectorAuditIssueSeverity;
import com.tuoman.ai_task_orchestrator.enums.VectorAuditIssueType;
import com.tuoman.ai_task_orchestrator.enums.VectorAuditRunStatus;
import com.tuoman.ai_task_orchestrator.enums.VectorAuditScopeType;
import com.tuoman.ai_task_orchestrator.repository.KnowledgeCollectionRepository;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreDocument;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.List;
import java.util.Map;
import java.util.Optional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class CollectionPollutionAuditServiceTest {

    @Mock
    private VectorConsistencyAuditService vectorConsistencyAuditService;

    @Mock
    private KnowledgeCollectionRepository knowledgeCollectionRepository;

    private CollectionPollutionAuditService service;

    @BeforeEach
    void setUp() {
        service = new CollectionPollutionAuditService(
                vectorConsistencyAuditService,
                knowledgeCollectionRepository
        );
    }

    @Test
    void auditCollectionShouldReturnHealthyWhenNoIssues() {
        KnowledgeCollectionEntity collection = collection(1L, "研发资料");
        VectorAuditRunEntity run = auditRun(10L);
        when(knowledgeCollectionRepository.findById(1L)).thenReturn(Optional.of(collection));
        when(vectorConsistencyAuditService.runAudit(VectorAuditScopeType.COLLECTION, 1L, null)).thenReturn(run);
        when(vectorConsistencyAuditService.findIssues(10L)).thenReturn(List.of());
        when(vectorConsistencyAuditService.scanVectorsForCollection(1L)).thenReturn(List.of(sampleVector(1L)));

        CollectionPollutionAuditService.CollectionPollutionAuditReport report = service.auditCollection(1L);

        assertThat(report.getCollectionId()).isEqualTo(1L);
        assertThat(report.getCollectionName()).isEqualTo("研发资料");
        assertThat(report.getStatus()).isEqualTo("HEALTHY");
        assertThat(report.getSuggestions()).contains("向量索引健康，无需立即处理");
    }

    @Test
    void auditCollectionShouldReturnCriticalWhenCrossCollectionLeakExists() {
        KnowledgeCollectionEntity collection = collection(1L, "研发资料");
        VectorAuditRunEntity run = auditRun(11L);
        VectorAuditIssueEntity issue = issue(VectorAuditIssueType.CROSS_COLLECTION_VECTOR_LEAK, VectorAuditIssueSeverity.CRITICAL);

        when(knowledgeCollectionRepository.findById(1L)).thenReturn(Optional.of(collection));
        when(vectorConsistencyAuditService.runAudit(VectorAuditScopeType.COLLECTION, 1L, null)).thenReturn(run);
        when(vectorConsistencyAuditService.findIssues(11L)).thenReturn(List.of(issue));
        when(vectorConsistencyAuditService.scanVectorsForCollection(1L)).thenReturn(List.of(
                sampleVector(1L),
                sampleVector(2L)
        ));

        CollectionPollutionAuditService.CollectionPollutionAuditReport report = service.auditCollection(1L);

        assertThat(report.getStatus()).isEqualTo("CRITICAL");
        assertThat(report.getCrossCollectionVectorLeakRate()).isGreaterThan(0);
        assertThat(report.getSuggestions()).anyMatch(s -> s.contains("cleanup-pollution"));
    }

    @Test
    void auditCollectionShouldReturnWarningWhenMissingVectorsExist() {
        KnowledgeCollectionEntity collection = collection(1L, "研发资料");
        VectorAuditRunEntity run = auditRun(12L);
        VectorAuditIssueEntity issue = issue(VectorAuditIssueType.MISSING_VECTOR, VectorAuditIssueSeverity.WARNING);

        when(knowledgeCollectionRepository.findById(1L)).thenReturn(Optional.of(collection));
        when(vectorConsistencyAuditService.runAudit(VectorAuditScopeType.COLLECTION, 1L, null)).thenReturn(run);
        when(vectorConsistencyAuditService.findIssues(12L)).thenReturn(List.of(issue));
        when(vectorConsistencyAuditService.scanVectorsForCollection(1L)).thenReturn(List.of(sampleVector(1L)));

        CollectionPollutionAuditService.CollectionPollutionAuditReport report = service.auditCollection(1L);

        assertThat(report.getStatus()).isEqualTo("WARNING");
        assertThat(report.getSuggestions()).anyMatch(s -> s.contains("重新索引"));
    }

    private KnowledgeCollectionEntity collection(Long id, String name) {
        KnowledgeCollectionEntity collection = new KnowledgeCollectionEntity();
        collection.setId(id);
        collection.setName(name);
        collection.setStatus(CollectionStatus.ACTIVE);
        return collection;
    }

    private VectorAuditRunEntity auditRun(Long id) {
        VectorAuditRunEntity run = new VectorAuditRunEntity();
        run.setId(id);
        run.setScopeType(VectorAuditScopeType.COLLECTION);
        run.setCollectionId(1L);
        run.setStatus(VectorAuditRunStatus.COMPLETED);
        return run;
    }

    private VectorAuditIssueEntity issue(VectorAuditIssueType type, VectorAuditIssueSeverity severity) {
        VectorAuditIssueEntity issue = new VectorAuditIssueEntity();
        issue.setIssueType(type);
        issue.setSeverity(severity);
        issue.setCollectionId(1L);
        issue.setMessage("test issue");
        return issue;
    }

    private VectorStoreDocument sampleVector(Long collectionId) {
        return new VectorStoreDocument(
                100L,
                10L,
                "content",
                VectorIndexTestFixtures.unitVector(128),
                "mock",
                "mock-embedding",
                128,
                "COSINE",
                Map.of("collection_id", String.valueOf(collectionId)),
                "vector-" + collectionId,
                "stable-" + collectionId,
                collectionId,
                "uid-1",
                1L
        );
    }
}
