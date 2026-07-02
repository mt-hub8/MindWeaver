package com.tuoman.ai_task_orchestrator.agent;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.tuoman.ai_task_orchestrator.entity.AgentTaskEventEntity;
import com.tuoman.ai_task_orchestrator.enums.AgentTaskEventStatus;
import com.tuoman.ai_task_orchestrator.enums.AgentTaskEventType;
import com.tuoman.ai_task_orchestrator.enums.AgentTaskStep;
import com.tuoman.ai_task_orchestrator.repository.AgentTaskEventRepository;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Propagation;
import org.springframework.transaction.annotation.Transactional;

import java.util.LinkedHashMap;
import java.util.Map;
import java.util.UUID;

@Slf4j
@Service
@RequiredArgsConstructor
public class AgentTaskEventRecorder {

    private final AgentTaskEventRepository agentTaskEventRepository;

    private final ObjectMapper objectMapper;

    public static String newTraceId() {
        return UUID.randomUUID().toString().replace("-", "");
    }

    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public void recordTaskCreated(Long taskId) {
        record(
                taskId,
                AgentTaskEventType.TASK_CREATED,
                AgentTaskStep.CREATED,
                AgentTaskEventStatus.COMPLETED,
                "Task created",
                "任务已创建，等待后台执行。",
                null,
                null,
                null,
                null,
                null
        );
    }

    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public void recordTaskQueued(Long taskId) {
        record(
                taskId,
                AgentTaskEventType.TASK_QUEUED,
                AgentTaskStep.QUEUED,
                AgentTaskEventStatus.COMPLETED,
                "Task queued",
                "任务已进入执行队列。",
                null,
                null,
                null,
                null,
                null
        );
    }

    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public void recordTaskStarted(Long taskId) {
        record(
                taskId,
                AgentTaskEventType.TASK_STARTED,
                AgentTaskStep.EXECUTION,
                AgentTaskEventStatus.STARTED,
                "Task started",
                "任务开始执行，系统将检索知识库并生成结果。",
                null,
                null,
                null,
                null,
                null
        );
    }

    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public void recordRetrievalStarted(Long taskId) {
        record(
                taskId,
                AgentTaskEventType.RETRIEVAL_STARTED,
                AgentTaskStep.RETRIEVAL,
                AgentTaskEventStatus.STARTED,
                "Retrieval started",
                "开始检索相关知识库内容。",
                null,
                null,
                null,
                null,
                null
        );
    }

    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public void recordRetrievalCompleted(Long taskId, int citationCount, long durationMs) {
        record(
                taskId,
                AgentTaskEventType.RETRIEVAL_COMPLETED,
                AgentTaskStep.RETRIEVAL,
                AgentTaskEventStatus.COMPLETED,
                "Retrieval completed",
                citationCount > 0
                        ? "知识库检索完成，已找到相关引用片段。"
                        : "知识库检索完成，当前范围内没有可用于任务的文档片段。",
                durationMs,
                metadata("citationCount", citationCount),
                null,
                null,
                null
        );
    }

    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public void recordLlmStarted(Long taskId) {
        record(
                taskId,
                AgentTaskEventType.LLM_STARTED,
                AgentTaskStep.LLM,
                AgentTaskEventStatus.STARTED,
                "LLM started",
                "开始调用大模型生成任务结果。",
                null,
                null,
                null,
                null,
                null
        );
    }

    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public void recordLlmCompleted(Long taskId, long durationMs, Map<String, Object> metadata) {
        record(
                taskId,
                AgentTaskEventType.LLM_COMPLETED,
                AgentTaskStep.LLM,
                AgentTaskEventStatus.COMPLETED,
                "LLM completed",
                "大模型生成完成。",
                durationMs,
                metadata,
                null,
                null,
                null
        );
    }

    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public void recordTaskCompleted(Long taskId) {
        record(
                taskId,
                AgentTaskEventType.TASK_COMPLETED,
                AgentTaskStep.FINALIZE,
                AgentTaskEventStatus.COMPLETED,
                "Task completed",
                "任务执行完成。",
                null,
                null,
                null,
                null,
                null
        );
    }

    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public void recordTaskFailed(
            Long taskId,
            AgentTaskStep step,
            String errorCode,
            String errorMessage,
            String traceId,
            Throwable cause
    ) {
        if (cause != null) {
            log.warn("Agent task failed, taskId={}, errorCode={}", taskId, errorCode, cause);
        }
        record(
                taskId,
                AgentTaskEventType.TASK_FAILED,
                step,
                AgentTaskEventStatus.FAILED,
                "Task failed",
                "任务执行失败，请查看错误原因。",
                null,
                null,
                errorCode,
                errorMessage,
                traceId
        );
    }

    private void record(
            Long taskId,
            AgentTaskEventType eventType,
            AgentTaskStep step,
            AgentTaskEventStatus status,
            String message,
            String displayMessage,
            Long durationMs,
            Map<String, Object> metadata,
            String errorCode,
            String errorMessage,
            String traceId
    ) {
        AgentTaskEventEntity entity = new AgentTaskEventEntity();
        entity.setTaskId(taskId);
        entity.setEventType(eventType);
        entity.setStep(step);
        entity.setStatus(status);
        entity.setMessage(message);
        entity.setDisplayMessage(displayMessage);
        entity.setDurationMs(durationMs);
        entity.setMetadataJson(serializeMetadata(metadata));
        entity.setErrorCode(errorCode);
        entity.setErrorMessage(errorMessage);
        entity.setTraceId(traceId);
        agentTaskEventRepository.save(entity);
    }

    private String serializeMetadata(Map<String, Object> metadata) {
        if (metadata == null || metadata.isEmpty()) {
            return null;
        }
        try {
            return objectMapper.writeValueAsString(metadata);
        } catch (JsonProcessingException exception) {
            log.warn("Failed to serialize agent task event metadata", exception);
            return null;
        }
    }

    private Map<String, Object> metadata(Object... keyValues) {
        Map<String, Object> metadata = new LinkedHashMap<>();
        for (int index = 0; index + 1 < keyValues.length; index += 2) {
            metadata.put(String.valueOf(keyValues[index]), keyValues[index + 1]);
        }
        return metadata;
    }
}
