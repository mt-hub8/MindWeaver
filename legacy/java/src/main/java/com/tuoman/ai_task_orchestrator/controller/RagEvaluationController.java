package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.dto.kbhealth.CompareRagEvaluationRunsRequest;
import com.tuoman.ai_task_orchestrator.dto.kbhealth.CompareRagEvaluationRunsResponse;
import com.tuoman.ai_task_orchestrator.dto.kbhealth.CreateRagEvaluationDatasetRequest;
import com.tuoman.ai_task_orchestrator.dto.kbhealth.CreateRagEvaluationRunRequest;
import com.tuoman.ai_task_orchestrator.dto.kbhealth.ImportRagEvaluationCasesRequest;
import com.tuoman.ai_task_orchestrator.dto.kbhealth.ImportRagEvaluationCasesResponse;
import com.tuoman.ai_task_orchestrator.dto.kbhealth.RagEvaluationCaseResponse;
import com.tuoman.ai_task_orchestrator.dto.kbhealth.RagEvaluationCaseResultResponse;
import com.tuoman.ai_task_orchestrator.dto.kbhealth.RagEvaluationDatasetResponse;
import com.tuoman.ai_task_orchestrator.dto.kbhealth.RagEvaluationRunResponse;
import com.tuoman.ai_task_orchestrator.dto.kbhealth.UpdateRagEvaluationCaseRequest;
import com.tuoman.ai_task_orchestrator.service.kbhealth.RagEvaluationDatasetService;
import com.tuoman.ai_task_orchestrator.service.kbhealth.RagEvaluationRunService;
import lombok.RequiredArgsConstructor;
import org.springframework.http.HttpStatus;
import org.springframework.web.bind.annotation.DeleteMapping;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.PutMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.ResponseStatus;
import org.springframework.web.bind.annotation.RestController;

import java.util.List;

@RestController
@RequestMapping("/rag/evaluation")
@RequiredArgsConstructor
public class RagEvaluationController {

    private final RagEvaluationDatasetService datasetService;

    private final RagEvaluationRunService runService;

    @PostMapping("/datasets")
    @ResponseStatus(HttpStatus.CREATED)
    public RagEvaluationDatasetResponse createDataset(@RequestBody CreateRagEvaluationDatasetRequest request) {
        return datasetService.createDataset(request);
    }

    @GetMapping("/datasets")
    public List<RagEvaluationDatasetResponse> listDatasets() {
        return datasetService.listDatasets();
    }

    @GetMapping("/datasets/{datasetId}")
    public RagEvaluationDatasetResponse getDataset(@PathVariable Long datasetId) {
        return datasetService.getDataset(datasetId);
    }

    @PostMapping("/datasets/{datasetId}/cases/import")
    public ImportRagEvaluationCasesResponse importCases(
            @PathVariable Long datasetId,
            @RequestBody ImportRagEvaluationCasesRequest request
    ) {
        return datasetService.importCases(datasetId, request);
    }

    @GetMapping("/datasets/{datasetId}/cases")
    public List<RagEvaluationCaseResponse> listCases(@PathVariable Long datasetId) {
        return datasetService.listCases(datasetId);
    }

    @PutMapping("/cases/{caseId}")
    public RagEvaluationCaseResponse updateCase(
            @PathVariable Long caseId,
            @RequestBody UpdateRagEvaluationCaseRequest request
    ) {
        return datasetService.updateCase(caseId, request);
    }

    @DeleteMapping("/cases/{caseId}")
    @ResponseStatus(HttpStatus.NO_CONTENT)
    public void deleteCase(@PathVariable Long caseId) {
        datasetService.deleteCase(caseId);
    }

    @PostMapping("/runs")
    @ResponseStatus(HttpStatus.CREATED)
    public RagEvaluationRunResponse createRun(@RequestBody CreateRagEvaluationRunRequest request) {
        return runService.createAndExecuteRun(request);
    }

    @GetMapping("/runs")
    public List<RagEvaluationRunResponse> listRuns() {
        return runService.listRuns();
    }

    @GetMapping("/runs/{runId}")
    public RagEvaluationRunResponse getRun(@PathVariable Long runId) {
        return runService.getRun(runId);
    }

    @GetMapping("/runs/{runId}/cases")
    public List<RagEvaluationCaseResultResponse> listRunCases(@PathVariable Long runId) {
        return runService.listCaseResults(runId);
    }

    @PostMapping("/runs/{runId}/cancel")
    public RagEvaluationRunResponse cancelRun(@PathVariable Long runId) {
        return runService.cancelRun(runId);
    }

    @PostMapping("/runs/compare")
    public CompareRagEvaluationRunsResponse compareRuns(@RequestBody CompareRagEvaluationRunsRequest request) {
        return runService.compareRuns(request);
    }
}
