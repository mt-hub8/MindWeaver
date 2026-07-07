package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.common.error.GlobalExceptionHandler;
import com.tuoman.ai_task_orchestrator.entity.VectorAuditRunEntity;
import com.tuoman.ai_task_orchestrator.enums.VectorAuditRunStatus;
import com.tuoman.ai_task_orchestrator.enums.VectorAuditScopeType;
import com.tuoman.ai_task_orchestrator.vectorindex.CollectionPollutionAuditService;
import com.tuoman.ai_task_orchestrator.vectorindex.VectorCleanupService;
import com.tuoman.ai_task_orchestrator.vectorindex.VectorConsistencyAuditService;
import com.tuoman.ai_task_orchestrator.vectorindex.VectorIndexSummaryService;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.context.annotation.Import;
import org.springframework.http.MediaType;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.test.web.servlet.MockMvc;

import java.time.LocalDateTime;
import java.util.List;

import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@WebMvcTest(controllers = VectorIndexHealthController.class)
@Import(GlobalExceptionHandler.class)
class VectorIndexHealthControllerTest {

    @Autowired
    private MockMvc mockMvc;

    @MockitoBean
    private VectorIndexSummaryService summaryService;

    @MockitoBean
    private VectorConsistencyAuditService auditService;

    @MockitoBean
    private CollectionPollutionAuditService collectionPollutionAuditService;

    @MockitoBean
    private VectorCleanupService cleanupService;

    @Test
    void summaryShouldReturnVectorCounts() throws Exception {
        when(summaryService.getSummary()).thenReturn(VectorIndexSummaryService.VectorIndexSummary.builder()
                .totalVectors(100)
                .totalChunks(95)
                .collectionCount(3)
                .activeGenerationCount(2)
                .build());

        mockMvc.perform(get("/vector-index/summary"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.totalVectors").value(100))
                .andExpect(jsonPath("$.totalChunks").value(95))
                .andExpect(jsonPath("$.collectionCount").value(3));
    }

    @Test
    void auditShouldCreateRun() throws Exception {
        when(auditService.runAudit(eq(VectorAuditScopeType.COLLECTION), eq(1L), eq(null)))
                .thenReturn(auditRun(5L, VectorAuditScopeType.COLLECTION));

        mockMvc.perform(post("/vector-index/audit")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("""
                                {"scopeType":"COLLECTION","collectionId":1}
                                """))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.id").value(5))
                .andExpect(jsonPath("$.scopeType").value("COLLECTION"));
    }

    @Test
    void collectionAuditShouldReturnPollutionReport() throws Exception {
        when(collectionPollutionAuditService.auditCollection(1L))
                .thenReturn(CollectionPollutionAuditService.CollectionPollutionAuditReport.builder()
                        .collectionId(1L)
                        .collectionName("研发资料")
                        .auditRunId(8L)
                        .totalVectors(10)
                        .status("HEALTHY")
                        .suggestions(List.of("向量索引健康，无需立即处理"))
                        .build());

        mockMvc.perform(post("/vector-index/collections/1/audit"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.collectionName").value("研发资料"))
                .andExpect(jsonPath("$.status").value("HEALTHY"));
    }

    @Test
    void cleanupOrphansShouldRequireCollectionId() throws Exception {
        mockMvc.perform(post("/vector-index/cleanup/orphans")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{}"))
                .andExpect(status().isBadRequest());
    }

    @Test
    void cleanupOrphansShouldReturnDeletedCount() throws Exception {
        when(cleanupService.cleanupOrphanVectors(1L)).thenReturn(
                VectorCleanupService.VectorCleanupResult.builder()
                        .requestedCount(3)
                        .deletedCount(2)
                        .warnings(List.of())
                        .build()
        );

        mockMvc.perform(post("/vector-index/cleanup/orphans")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("""
                                {"collectionId":1}
                                """))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.deletedCount").value(2));
    }

    private VectorAuditRunEntity auditRun(Long id, VectorAuditScopeType scopeType) {
        VectorAuditRunEntity run = new VectorAuditRunEntity();
        run.setId(id);
        run.setScopeType(scopeType);
        run.setCollectionId(1L);
        run.setStatus(VectorAuditRunStatus.COMPLETED);
        run.setStartedAt(LocalDateTime.now());
        run.setCompletedAt(LocalDateTime.now());
        run.setSummaryJson("{}");
        return run;
    }
}
