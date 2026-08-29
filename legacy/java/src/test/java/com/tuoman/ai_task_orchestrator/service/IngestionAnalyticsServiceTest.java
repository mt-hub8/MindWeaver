package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.dto.IngestionAnalyticsResponse;
import com.tuoman.ai_task_orchestrator.dto.IngestionFailureReasonResponse;
import com.tuoman.ai_task_orchestrator.entity.DocumentIngestionEventEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentIngestionTaskEntity;
import com.tuoman.ai_task_orchestrator.enums.IngestionEventStatus;
import com.tuoman.ai_task_orchestrator.enums.IngestionEventType;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStatus;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStep;
import com.tuoman.ai_task_orchestrator.repository.DocumentIngestionEventRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentIngestionTaskRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.time.LocalDateTime;
import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class IngestionAnalyticsServiceTest {

    @Mock
    private DocumentIngestionTaskRepository documentIngestionTaskRepository;

    @Mock
    private DocumentIngestionEventRepository documentIngestionEventRepository;

    private IngestionAnalyticsService ingestionAnalyticsService;

    @BeforeEach
    void setUp() {
        ingestionAnalyticsService = new IngestionAnalyticsService(
                documentIngestionTaskRepository,
                documentIngestionEventRepository
        );
    }

    @Test
    void getAnalyticsShouldReturnSafeEmptyResult() {
        when(documentIngestionTaskRepository.findByCreatedAtGreaterThanEqual(any())).thenReturn(List.of());

        IngestionAnalyticsResponse response = ingestionAnalyticsService.getAnalytics("24h");

        assertThat(response.getTotalTasks()).isZero();
        assertThat(response.getSuccessRate()).isZero();
        assertThat(response.getFailureRate()).isZero();
        assertThat(response.getAverageTotalDurationMs()).isNull();
        assertThat(response.getStageDurations()).hasSize(3);
        assertThat(response.getTopFailureReasons()).isEmpty();
    }

    @Test
    void getAnalyticsShouldCalculateCountsRatesAndAverageDuration() {
        DocumentIngestionTaskEntity completed = completedTask(
                1L,
                LocalDateTime.of(2026, 7, 2, 10, 0, 0),
                LocalDateTime.of(2026, 7, 2, 10, 0, 5)
        );
        DocumentIngestionTaskEntity failed = task(2L, IngestionTaskStatus.FAILED);
        failed.setErrorCode("EMBEDDING_PROVIDER_ERROR");
        failed.setCompletedAt(LocalDateTime.of(2026, 7, 2, 11, 0, 0));
        DocumentIngestionTaskEntity pending = task(3L, IngestionTaskStatus.PENDING);
        DocumentIngestionTaskEntity processing = task(4L, IngestionTaskStatus.PROCESSING);

        when(documentIngestionTaskRepository.findByCreatedAtGreaterThanEqual(any()))
                .thenReturn(List.of(completed, failed, pending, processing));
        when(documentIngestionEventRepository.findByTaskIdInAndEventTypeIn(any(), any()))
                .thenReturn(List.of(
                        stageEvent(1L, IngestionEventType.CHUNKING_COMPLETED, 100L),
                        stageEvent(1L, IngestionEventType.EMBEDDING_COMPLETED, 300L),
                        stageEvent(1L, IngestionEventType.VECTOR_WRITE_COMPLETED, 200L)
                ));
        when(documentIngestionEventRepository.findByTaskIdInAndEventType(any(), eq(IngestionEventType.TASK_FAILED)))
                .thenReturn(List.of(failedEvent(2L, "EMBEDDING_PROVIDER_ERROR", "向量失败")));

        IngestionAnalyticsResponse response = ingestionAnalyticsService.getAnalytics("24h");

        assertThat(response.getTotalTasks()).isEqualTo(4);
        assertThat(response.getCompletedTasks()).isEqualTo(1);
        assertThat(response.getFailedTasks()).isEqualTo(1);
        assertThat(response.getPendingTasks()).isEqualTo(1);
        assertThat(response.getProcessingTasks()).isEqualTo(1);
        assertThat(response.getSuccessRate()).isEqualTo(0.25);
        assertThat(response.getFailureRate()).isEqualTo(0.25);
        assertThat(response.getAverageTotalDurationMs()).isEqualTo(5000L);
        assertThat(response.getStageDurations().get(0).getDisplayName()).isEqualTo("文档切分");
        assertThat(response.getStageDurations().get(0).getAverageDurationMs()).isEqualTo(100L);
        assertThat(response.getTopFailureReasons()).extracting(IngestionFailureReasonResponse::getErrorCode)
                .containsExactly("EMBEDDING_PROVIDER_ERROR");
        assertThat(response.getRecentFailures()).hasSize(1);
        assertThat(response.getSlowTasks()).hasSize(1);
        assertThat(response.getSlowTasks().get(0).getBottleneckStage()).isEqualTo("EMBEDDING");
        assertThat(response.getSlowTasks().get(0).getBottleneckDisplayName()).isEqualTo("生成文档向量");
    }

    @Test
    void getAnalyticsAllWindowShouldUseFindAll() {
        when(documentIngestionTaskRepository.findAll()).thenReturn(List.of());

        ingestionAnalyticsService.getAnalytics("all");

        org.mockito.Mockito.verify(documentIngestionTaskRepository).findAll();
    }

    @Test
    void getAnalyticsShouldRejectInvalidWindow() {
        assertThatThrownBy(() -> ingestionAnalyticsService.getAnalytics("invalid"))
                .isInstanceOf(BusinessException.class);
    }

    @Test
    void calculateAverageTotalDurationShouldIgnoreTasksWithoutCompletedAt() {
        DocumentIngestionTaskEntity completed = completedTask(
                1L,
                LocalDateTime.of(2026, 7, 2, 10, 0, 0),
                LocalDateTime.of(2026, 7, 2, 10, 0, 2)
        );
        DocumentIngestionTaskEntity missingCompletedAt = task(2L, IngestionTaskStatus.COMPLETED);

        Long average = IngestionAnalyticsService.calculateAverageTotalDuration(List.of(completed, missingCompletedAt));

        assertThat(average).isEqualTo(2000L);
    }

    @Test
    void buildTopFailureReasonsShouldGroupUnknownError() {
        List<IngestionFailureReasonResponse> reasons = IngestionAnalyticsService.buildTopFailureReasons(List.of(
                failedEvent(1L, null, "无错误码"),
                failedEvent(2L, "", "空错误码")
        ));

        assertThat(reasons).hasSize(1);
        assertThat(reasons.get(0).getErrorCode()).isEqualTo("UNKNOWN_ERROR");
        assertThat(reasons.get(0).getDisplayMessage()).isEqualTo("未知错误");
        assertThat(reasons.get(0).getCount()).isEqualTo(2);
    }

    private DocumentIngestionTaskEntity task(Long id, IngestionTaskStatus status) {
        DocumentIngestionTaskEntity task = new DocumentIngestionTaskEntity();
        task.setId(id);
        task.setDocumentId(id);
        task.setFilename("demo-" + id + ".txt");
        task.setStatus(status);
        task.setStep(IngestionTaskStep.TEXT_EXTRACTED);
        task.setSourceText("content");
        task.setChunkCount(0);
        task.setEmbeddingCount(0);
        task.setVectorWriteCount(0);
        task.setRetryCount(0);
        task.setCreatedAt(LocalDateTime.of(2026, 7, 2, 9, 0, 0));
        task.setUpdatedAt(LocalDateTime.of(2026, 7, 2, 9, 30, 0));
        return task;
    }

    private DocumentIngestionTaskEntity completedTask(Long id, LocalDateTime createdAt, LocalDateTime completedAt) {
        DocumentIngestionTaskEntity task = task(id, IngestionTaskStatus.COMPLETED);
        task.setCreatedAt(createdAt);
        task.setCompletedAt(completedAt);
        task.setUpdatedAt(completedAt);
        return task;
    }

    private DocumentIngestionEventEntity stageEvent(Long taskId, IngestionEventType type, long durationMs) {
        DocumentIngestionEventEntity event = new DocumentIngestionEventEntity();
        event.setTaskId(taskId);
        event.setEventType(type);
        event.setStatus(IngestionEventStatus.COMPLETED);
        event.setDisplayMessage("stage");
        event.setDurationMs(durationMs);
        return event;
    }

    private DocumentIngestionEventEntity failedEvent(Long taskId, String errorCode, String errorMessage) {
        DocumentIngestionEventEntity event = new DocumentIngestionEventEntity();
        event.setTaskId(taskId);
        event.setEventType(IngestionEventType.TASK_FAILED);
        event.setStatus(IngestionEventStatus.FAILED);
        event.setDisplayMessage("failed");
        event.setErrorCode(errorCode);
        event.setErrorMessage(errorMessage);
        return event;
    }
}
