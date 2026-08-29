package com.tuoman.ai_task_orchestrator.controller;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.tuoman.ai_task_orchestrator.common.error.GlobalExceptionHandler;
import com.tuoman.ai_task_orchestrator.dto.kbhealth.CompareRagEvaluationRunsResponse;
import com.tuoman.ai_task_orchestrator.dto.kbhealth.CreateRagEvaluationRunRequest;
import com.tuoman.ai_task_orchestrator.dto.kbhealth.RagEvaluationCaseResultResponse;
import com.tuoman.ai_task_orchestrator.dto.kbhealth.RagEvaluationRunResponse;
import com.tuoman.ai_task_orchestrator.kbhealth.RagEvaluationRetrievalStrategy;
import com.tuoman.ai_task_orchestrator.kbhealth.RagEvaluationRunStatus;
import com.tuoman.ai_task_orchestrator.kbhealth.RagHealthScoringProfile;
import com.tuoman.ai_task_orchestrator.service.kbhealth.RagEvaluationDatasetService;
import com.tuoman.ai_task_orchestrator.service.kbhealth.RagEvaluationRunService;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.context.annotation.Import;
import org.springframework.http.MediaType;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.test.web.servlet.MockMvc;

import java.time.LocalDateTime;
import java.util.List;
import java.util.Map;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@WebMvcTest(controllers = RagEvaluationController.class)
@Import(GlobalExceptionHandler.class)
class EvaluationRunControllerTest {

    @Autowired
    private MockMvc mockMvc;

    @Autowired
    private ObjectMapper objectMapper;

    @MockitoBean
    private RagEvaluationDatasetService datasetService;

    @MockitoBean
    private RagEvaluationRunService runService;

    @Test
    void createRunShouldReturnCreatedRun() throws Exception {
        when(runService.createAndExecuteRun(any())).thenReturn(sampleRun(1L));

        CreateRagEvaluationRunRequest request = new CreateRagEvaluationRunRequest();
        request.setDatasetId(1L);
        request.setStrategy(RagEvaluationRetrievalStrategy.VECTOR_ONLY);
        request.setScoringProfile(RagHealthScoringProfile.BALANCED);
        request.setTopK(5);
        request.setExecuteGeneration(false);

        mockMvc.perform(post("/rag/evaluation/runs")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(objectMapper.writeValueAsString(request)))
                .andExpect(status().isCreated())
                .andExpect(jsonPath("$.runId").value(1))
                .andExpect(jsonPath("$.overallScore").value(74));
    }

    @Test
    void getRunAndCasesShouldWork() throws Exception {
        when(runService.getRun(1L)).thenReturn(sampleRun(1L));
        when(runService.listCaseResults(1L)).thenReturn(List.of(
                new RagEvaluationCaseResultResponse(
                        1L, 1L, "c1", "query", RagEvaluationRetrievalStrategy.VECTOR_ONLY, 5,
                        List.of(), "answer", List.of(), List.of(), List.of(), 70, Map.of(), 10L, null, null
                )
        ));

        mockMvc.perform(get("/rag/evaluation/runs/1"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.status").value("COMPLETED"));

        mockMvc.perform(get("/rag/evaluation/runs/1/cases"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$[0].caseId").value("c1"));
    }

    @Test
    void compareRunsShouldReturnDelta() throws Exception {
        when(runService.compareRuns(any())).thenReturn(
                new CompareRagEvaluationRunsResponse(70, 82, 12, Map.of("RECALL_AT_K", 0.12), List.of("RECALL_AT_K"), List.of(), "提升", List.of())
        );

        mockMvc.perform(post("/rag/evaluation/runs/compare")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"baselineRunId\":1,\"candidateRunId\":2}"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.deltaScore").value(12));
    }

    private RagEvaluationRunResponse sampleRun(Long id) {
        return new RagEvaluationRunResponse(
                id,
                1L,
                "run",
                RagEvaluationRunStatus.COMPLETED,
                RagEvaluationRetrievalStrategy.VECTOR_ONLY,
                "纯向量检索",
                RagHealthScoringProfile.BALANCED,
                "平衡模式",
                5,
                10,
                null,
                null,
                Map.of(),
                false,
                LocalDateTime.now(),
                LocalDateTime.now(),
                1,
                1,
                0,
                74,
                "一般",
                Map.of(),
                Map.of("summary", "ok")
        );
    }
}
