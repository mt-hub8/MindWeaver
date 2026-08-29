package com.tuoman.ai_task_orchestrator.service.kbhealth;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.tuoman.ai_task_orchestrator.dto.kbhealth.CreateRagEvaluationDatasetRequest;
import com.tuoman.ai_task_orchestrator.dto.kbhealth.ImportRagEvaluationCasesRequest;
import com.tuoman.ai_task_orchestrator.entity.RagEvaluationCaseEntity;
import com.tuoman.ai_task_orchestrator.entity.RagEvaluationDatasetEntity;
import com.tuoman.ai_task_orchestrator.kbhealth.RagEvaluationDatasetStatus;
import com.tuoman.ai_task_orchestrator.kbhealth.RagEvaluationDatasetType;
import com.tuoman.ai_task_orchestrator.repository.RagEvaluationCaseRepository;
import com.tuoman.ai_task_orchestrator.repository.RagEvaluationDatasetRepository;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.ArgumentCaptor;
import org.mockito.InjectMocks;
import org.mockito.Mock;
import org.mockito.Spy;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.List;
import java.util.Optional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class RagEvaluationDatasetServiceTest {

    @Mock
    private RagEvaluationDatasetRepository datasetRepository;

    @Mock
    private RagEvaluationCaseRepository caseRepository;

    @Spy
    private ObjectMapper objectMapper = new ObjectMapper();

    @InjectMocks
    private RagEvaluationDatasetService service;

    @Test
    void createDatasetShouldPersistEntity() {
        when(datasetRepository.save(any())).thenAnswer(inv -> {
            RagEvaluationDatasetEntity entity = inv.getArgument(0);
            entity.setId(1L);
            return entity;
        });

        CreateRagEvaluationDatasetRequest request = new CreateRagEvaluationDatasetRequest();
        request.setName("Gold Test Set v1");
        request.setDatasetType(RagEvaluationDatasetType.GOLD_TEST_SET);

        var response = service.createDataset(request);
        assertThat(response.getDatasetId()).isEqualTo(1L);
        assertThat(response.getName()).isEqualTo("Gold Test Set v1");
    }

    @Test
    void importJsonCasesShouldAllowMissingExpectedFields() {
        RagEvaluationDatasetEntity dataset = new RagEvaluationDatasetEntity();
        dataset.setId(1L);
        when(datasetRepository.findById(1L)).thenReturn(Optional.of(dataset));
        when(caseRepository.findByDatasetIdAndCaseId(any(), any())).thenReturn(Optional.empty());
        when(caseRepository.save(any())).thenAnswer(inv -> inv.getArgument(0));
        when(datasetRepository.save(any())).thenReturn(dataset);
        when(caseRepository.countByDatasetIdAndEnabledTrue(1L)).thenReturn(1L);

        ImportRagEvaluationCasesRequest request = new ImportRagEvaluationCasesRequest();
        request.setFormat("json");
        request.setPayload("[{\"query\":\"测试问题\"}]");

        var result = service.importCases(1L, request);
        assertThat(result.getImportedCount()).isEqualTo(1);
        assertThat(result.getFailedCount()).isZero();

        ArgumentCaptor<RagEvaluationCaseEntity> captor = ArgumentCaptor.forClass(RagEvaluationCaseEntity.class);
        verify(caseRepository).save(captor.capture());
        assertThat(captor.getValue().getExpectedChunkIdsJson()).isNull();
    }

    @Test
    void importShouldReportInvalidCaseClearly() {
        RagEvaluationDatasetEntity dataset = new RagEvaluationDatasetEntity();
        dataset.setId(1L);
        when(datasetRepository.findById(1L)).thenReturn(Optional.of(dataset));
        when(datasetRepository.save(any())).thenReturn(dataset);

        ImportRagEvaluationCasesRequest request = new ImportRagEvaluationCasesRequest();
        request.setPayload("[{\"case_id\":\"c1\"}]");

        var result = service.importCases(1L, request);
        assertThat(result.getFailedCount()).isEqualTo(1);
        assertThat(result.getErrors().get(0)).contains("query is required");
    }

    @Test
    void listCasesShouldReturnSavedCases() {
        RagEvaluationDatasetEntity dataset = new RagEvaluationDatasetEntity();
        dataset.setId(1L);
        when(datasetRepository.findById(1L)).thenReturn(Optional.of(dataset));

        RagEvaluationCaseEntity entity = new RagEvaluationCaseEntity();
        entity.setId(10L);
        entity.setDatasetId(1L);
        entity.setCaseId("c1");
        entity.setQuery("q");
        entity.setEnabled(true);
        when(caseRepository.findByDatasetIdOrderByIdAsc(1L)).thenReturn(List.of(entity));

        assertThat(service.listCases(1L)).hasSize(1);
    }
}
