package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.document.ingestion.IngestionAnalyticsDisplayTexts;
import com.tuoman.ai_task_orchestrator.document.ingestion.IngestionAnalyticsWindow;
import com.tuoman.ai_task_orchestrator.dto.IngestionAnalyticsResponse;
import com.tuoman.ai_task_orchestrator.dto.IngestionFailureReasonResponse;
import com.tuoman.ai_task_orchestrator.dto.IngestionRecentFailureResponse;
import com.tuoman.ai_task_orchestrator.dto.IngestionSlowTaskResponse;
import com.tuoman.ai_task_orchestrator.dto.IngestionStageDurationResponse;
import com.tuoman.ai_task_orchestrator.entity.DocumentIngestionEventEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentIngestionTaskEntity;
import com.tuoman.ai_task_orchestrator.enums.IngestionEventType;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentIngestionEventRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentIngestionTaskRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.Duration;
import java.time.LocalDateTime;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.Set;
import java.util.stream.Collectors;

@Service
@RequiredArgsConstructor
/**
 * V3.5 ingestion 性能和失败分析服务。
 *
 * 基于 ingestion task/event 聚合成功率、失败原因、阶段耗时和慢任务。
 * 这些结果只用于诊断摄入链路，不应改变文档状态、重试策略或 vector 写入结果。
 */
public class IngestionAnalyticsService {

    private static final int TOP_FAILURE_LIMIT = 5;

    private static final int RECENT_FAILURE_LIMIT = 5;

    private static final int SLOW_TASK_LIMIT = 5;

    private static final List<IngestionEventType> STAGE_COMPLETED_EVENT_TYPES = List.of(
            IngestionEventType.CHUNKING_COMPLETED,
            IngestionEventType.EMBEDDING_COMPLETED,
            IngestionEventType.VECTOR_WRITE_COMPLETED
    );

    private final DocumentIngestionTaskRepository documentIngestionTaskRepository;

    private final DocumentIngestionEventRepository documentIngestionEventRepository;

    @Transactional(readOnly = true)
    public IngestionAnalyticsResponse getAnalytics(String windowParam) {
        // 统计窗口只限制诊断样本范围。
        // 指标缺失时保持 null/空列表，而不是伪造阶段耗时或失败原因。
        IngestionAnalyticsWindow window = IngestionAnalyticsWindow.parse(windowParam);
        LocalDateTime now = LocalDateTime.now();
        List<DocumentIngestionTaskEntity> tasks = loadTasks(window, now);

        int totalTasks = tasks.size();
        int completedTasks = countByStatus(tasks, IngestionTaskStatus.COMPLETED);
        int failedTasks = countByStatus(tasks, IngestionTaskStatus.FAILED);
        int processingTasks = countByStatus(tasks, IngestionTaskStatus.PROCESSING);
        int pendingTasks = countByStatus(tasks, IngestionTaskStatus.PENDING);

        double successRate = totalTasks == 0 ? 0.0 : (double) completedTasks / totalTasks;
        double failureRate = totalTasks == 0 ? 0.0 : (double) failedTasks / totalTasks;

        List<DocumentIngestionTaskEntity> completedTaskEntities = tasks.stream()
                .filter(task -> task.getStatus() == IngestionTaskStatus.COMPLETED)
                .toList();
        Long averageTotalDurationMs = calculateAverageTotalDuration(completedTaskEntities);

        Set<Long> taskIds = tasks.stream().map(DocumentIngestionTaskEntity::getId).collect(Collectors.toSet());
        List<DocumentIngestionEventEntity> stageEvents = taskIds.isEmpty()
                ? List.of()
                : documentIngestionEventRepository.findByTaskIdInAndEventTypeIn(taskIds, STAGE_COMPLETED_EVENT_TYPES);
        List<IngestionStageDurationResponse> stageDurations = buildStageDurations(stageEvents);

        List<DocumentIngestionEventEntity> failedEvents = taskIds.isEmpty()
                ? List.of()
                : documentIngestionEventRepository.findByTaskIdInAndEventType(taskIds, IngestionEventType.TASK_FAILED);
        List<IngestionFailureReasonResponse> topFailureReasons = buildTopFailureReasons(failedEvents);
        List<IngestionRecentFailureResponse> recentFailures = buildRecentFailures(tasks);
        List<IngestionSlowTaskResponse> slowTasks = buildSlowTasks(completedTaskEntities, stageEvents);

        return new IngestionAnalyticsResponse(
                window.getCode(),
                window.getDisplayWindow(),
                totalTasks,
                completedTasks,
                failedTasks,
                processingTasks,
                pendingTasks,
                successRate,
                failureRate,
                averageTotalDurationMs,
                stageDurations,
                topFailureReasons,
                recentFailures,
                slowTasks
        );
    }

    private List<DocumentIngestionTaskEntity> loadTasks(IngestionAnalyticsWindow window, LocalDateTime now) {
        LocalDateTime since = window.resolveSince(now);
        if (since == null) {
            return documentIngestionTaskRepository.findAll();
        }
        return documentIngestionTaskRepository.findByCreatedAtGreaterThanEqual(since);
    }

    private int countByStatus(List<DocumentIngestionTaskEntity> tasks, IngestionTaskStatus status) {
        return (int) tasks.stream().filter(task -> task.getStatus() == status).count();
    }

    static Long calculateAverageTotalDuration(List<DocumentIngestionTaskEntity> completedTasks) {
        if (completedTasks.isEmpty()) {
            return null;
        }
        long totalDurationMs = 0;
        int counted = 0;
        for (DocumentIngestionTaskEntity task : completedTasks) {
            Long durationMs = calculateTotalDurationMs(task);
            if (durationMs != null) {
                totalDurationMs += durationMs;
                counted++;
            }
        }
        if (counted == 0) {
            return null;
        }
        return totalDurationMs / counted;
    }

    static Long calculateTotalDurationMs(DocumentIngestionTaskEntity task) {
        if (task.getCreatedAt() == null || task.getCompletedAt() == null) {
            return null;
        }
        return Duration.between(task.getCreatedAt(), task.getCompletedAt()).toMillis();
    }

    private List<IngestionStageDurationResponse> buildStageDurations(List<DocumentIngestionEventEntity> stageEvents) {
        Map<String, List<Long>> durationsByStage = new HashMap<>();
        for (IngestionEventType eventType : STAGE_COMPLETED_EVENT_TYPES) {
            durationsByStage.put(IngestionAnalyticsDisplayTexts.eventTypeToStage(eventType), new ArrayList<>());
        }

        for (DocumentIngestionEventEntity event : stageEvents) {
            String stage = IngestionAnalyticsDisplayTexts.eventTypeToStage(event.getEventType());
            if (stage == null || event.getDurationMs() == null) {
                continue;
            }
            durationsByStage.computeIfAbsent(stage, ignored -> new ArrayList<>()).add(event.getDurationMs());
        }

        return List.of("CHUNKING", "EMBEDDING", "VECTOR_WRITING").stream()
                .map(stage -> {
                    List<Long> durations = durationsByStage.getOrDefault(stage, List.of());
                    Long average = durations.isEmpty()
                            ? null
                            : durations.stream().mapToLong(Long::longValue).sum() / durations.size();
                    return new IngestionStageDurationResponse(
                            stage,
                            IngestionAnalyticsDisplayTexts.stageDisplayName(stage),
                            average,
                            durations.size()
                    );
                })
                .toList();
    }

    static List<IngestionFailureReasonResponse> buildTopFailureReasons(List<DocumentIngestionEventEntity> failedEvents) {
        // failure analytics 是归类和排序，不吞异常。
        // 原始失败仍保存在 task/event 中，便于用户追踪具体 traceId。
        Map<String, Integer> counts = new HashMap<>();
        Map<String, String> sampleMessages = new HashMap<>();
        for (DocumentIngestionEventEntity event : failedEvents) {
            String errorCode = normalizeErrorCode(event.getErrorCode());
            counts.merge(errorCode, 1, Integer::sum);
            sampleMessages.putIfAbsent(errorCode, event.getErrorMessage());
        }

        return counts.entrySet().stream()
                .sorted(Map.Entry.<String, Integer>comparingByValue().reversed())
                .limit(TOP_FAILURE_LIMIT)
                .map(entry -> new IngestionFailureReasonResponse(
                        entry.getKey(),
                        IngestionAnalyticsDisplayTexts.failureReasonDisplayMessage(
                                entry.getKey(),
                                sampleMessages.get(entry.getKey())
                        ),
                        entry.getValue()
                ))
                .toList();
    }

    private List<IngestionRecentFailureResponse> buildRecentFailures(List<DocumentIngestionTaskEntity> tasks) {
        return tasks.stream()
                .filter(task -> task.getStatus() == IngestionTaskStatus.FAILED)
                .sorted(Comparator.comparing(
                        this::resolveFailedAt,
                        Comparator.nullsLast(Comparator.reverseOrder())
                ))
                .limit(RECENT_FAILURE_LIMIT)
                .map(task -> new IngestionRecentFailureResponse(
                        task.getId(),
                        task.getDocumentId(),
                        task.getFilename(),
                        normalizeErrorCode(task.getErrorCode()),
                        task.getErrorMessage(),
                        IngestionAnalyticsDisplayTexts.failedTaskDisplayMessage(
                                task.getErrorCode(),
                                task.getErrorMessage()
                        ),
                        resolveFailedAt(task),
                        task.getRetryCount()
                ))
                .toList();
    }

    private List<IngestionSlowTaskResponse> buildSlowTasks(
            List<DocumentIngestionTaskEntity> completedTasks,
            List<DocumentIngestionEventEntity> stageEvents
    ) {
        Map<Long, List<DocumentIngestionEventEntity>> eventsByTaskId = stageEvents.stream()
                .collect(Collectors.groupingBy(DocumentIngestionEventEntity::getTaskId));

        return completedTasks.stream()
                .map(task -> {
                    Long totalDurationMs = calculateTotalDurationMs(task);
                    if (totalDurationMs == null) {
                        return null;
                    }
                    Bottleneck bottleneck = resolveBottleneck(eventsByTaskId.getOrDefault(task.getId(), List.of()));
                    return new IngestionSlowTaskResponse(
                            task.getId(),
                            task.getDocumentId(),
                            task.getFilename(),
                            totalDurationMs,
                            bottleneck.stage(),
                            bottleneck.displayName(),
                            task.getCreatedAt(),
                            task.getCompletedAt()
                    );
                })
                .filter(Objects::nonNull)
                .sorted(Comparator.comparing(IngestionSlowTaskResponse::getTotalDurationMs).reversed())
                .limit(SLOW_TASK_LIMIT)
                .toList();
    }

    static Bottleneck resolveBottleneck(List<DocumentIngestionEventEntity> stageEvents) {
        DocumentIngestionEventEntity slowest = stageEvents.stream()
                .filter(event -> STAGE_COMPLETED_EVENT_TYPES.contains(event.getEventType()))
                .filter(event -> event.getDurationMs() != null)
                .max(Comparator.comparing(DocumentIngestionEventEntity::getDurationMs))
                .orElse(null);
        if (slowest == null) {
            return new Bottleneck(null, null);
        }
        String stage = IngestionAnalyticsDisplayTexts.eventTypeToStage(slowest.getEventType());
        return new Bottleneck(stage, IngestionAnalyticsDisplayTexts.stageDisplayName(stage));
    }

    private LocalDateTime resolveFailedAt(DocumentIngestionTaskEntity task) {
        if (task.getCompletedAt() != null) {
            return task.getCompletedAt();
        }
        return task.getUpdatedAt();
    }

    static String normalizeErrorCode(String errorCode) {
        if (errorCode == null || errorCode.isBlank()) {
            return "UNKNOWN_ERROR";
        }
        return errorCode.trim();
    }

    record Bottleneck(String stage, String displayName) {
    }
}
