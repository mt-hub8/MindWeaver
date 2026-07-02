package com.tuoman.ai_task_orchestrator.document.ingestion;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.tuoman.ai_task_orchestrator.entity.DocumentIngestionEventEntity;
import com.tuoman.ai_task_orchestrator.enums.IngestionEventStatus;
import com.tuoman.ai_task_orchestrator.enums.IngestionEventType;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStep;
import com.tuoman.ai_task_orchestrator.repository.DocumentIngestionEventRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentIngestionEventRepository;
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
public class DocumentIngestionEventRecorder {

    private final DocumentIngestionEventRepository documentIngestionEventRepository;

    private final ObjectMapper objectMapper;

    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public void recordTaskCreated(Long taskId, String filename, Long documentId) {
        record(
                taskId,
                IngestionEventType.TASK_CREATED,
                IngestionTaskStep.UPLOADED,
                IngestionEventStatus.COMPLETED,
                "Task created",
                "文档已提交，系统已创建处理任务。",
                null,
                metadata("filename", filename, "documentId", documentId),
                null,
                null,
                null
        );
    }

    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public void recordTextExtracted(Long taskId, String filename) {
        record(
                taskId,
                IngestionEventType.TEXT_EXTRACTED,
                IngestionTaskStep.TEXT_EXTRACTED,
                IngestionEventStatus.COMPLETED,
                "Text extracted",
                "已读取文档文本，正在准备建立知识库索引。",
                null,
                metadata("filename", filename),
                null,
                null,
                null
        );
    }

    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public void recordTaskQueued(Long taskId) {
        record(
                taskId,
                IngestionEventType.TASK_QUEUED,
                IngestionTaskStep.TEXT_EXTRACTED,
                IngestionEventStatus.COMPLETED,
                "Task queued",
                "文档已进入处理队列，请稍候。",
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
                IngestionEventType.TASK_STARTED,
                IngestionTaskStep.CHUNKING,
                IngestionEventStatus.STARTED,
                "Task processing started",
                "开始处理文档，系统将切分内容并建立知识库索引。",
                null,
                null,
                null,
                null,
                null
        );
    }

    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public void recordChunkingStarted(Long taskId) {
        record(
                taskId,
                IngestionEventType.CHUNKING_STARTED,
                IngestionTaskStep.CHUNKING,
                IngestionEventStatus.STARTED,
                "Chunking started",
                "正在切分文档内容。",
                null,
                null,
                null,
                null,
                null
        );
    }

    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public void recordChunkingCompleted(Long taskId, int chunkCount, long durationMs) {
        record(
                taskId,
                IngestionEventType.CHUNKING_COMPLETED,
                IngestionTaskStep.CHUNKING,
                IngestionEventStatus.COMPLETED,
                "Chunking completed",
                "文档内容已切分完成，共生成 " + chunkCount + " 个文档片段（Chunk）。",
                durationMs,
                metadata("chunkCount", chunkCount),
                null,
                null,
                null
        );
    }

    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public void recordEmbeddingStarted(Long taskId) {
        record(
                taskId,
                IngestionEventType.EMBEDDING_STARTED,
                IngestionTaskStep.EMBEDDING,
                IngestionEventStatus.STARTED,
                "Embedding started",
                "正在生成文档向量。",
                null,
                null,
                null,
                null,
                null
        );
    }

    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public void recordEmbeddingCompleted(Long taskId, int embeddingCount, long durationMs) {
        record(
                taskId,
                IngestionEventType.EMBEDDING_COMPLETED,
                IngestionTaskStep.EMBEDDING,
                IngestionEventStatus.COMPLETED,
                "Embedding completed",
                "文档向量已生成完成，共生成 " + embeddingCount + " 个向量。",
                durationMs,
                metadata("embeddingCount", embeddingCount),
                null,
                null,
                null
        );
    }

    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public void recordVectorWriteStarted(Long taskId) {
        record(
                taskId,
                IngestionEventType.VECTOR_WRITE_STARTED,
                IngestionTaskStep.VECTOR_WRITING,
                IngestionEventStatus.STARTED,
                "Vector write started",
                "正在写入知识库索引（Vector Index）。",
                null,
                null,
                null,
                null,
                null
        );
    }

    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public void recordVectorWriteCompleted(Long taskId, int vectorWriteCount, long durationMs) {
        record(
                taskId,
                IngestionEventType.VECTOR_WRITE_COMPLETED,
                IngestionTaskStep.VECTOR_WRITING,
                IngestionEventStatus.COMPLETED,
                "Vector write completed",
                "文档向量已写入知识库索引，共写入 " + vectorWriteCount + " 条。",
                durationMs,
                metadata("vectorWriteCount", vectorWriteCount),
                null,
                null,
                null
        );
    }

    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public void recordTaskCompleted(Long taskId, long totalDurationMs) {
        Map<String, Object> metadata = new LinkedHashMap<>();
        metadata.put("totalDurationMs", totalDurationMs);
        record(
                taskId,
                IngestionEventType.TASK_COMPLETED,
                IngestionTaskStep.COMPLETED,
                IngestionEventStatus.COMPLETED,
                "Task completed",
                "处理完成，现在可以前往「知识库问答」页面提问。",
                totalDurationMs,
                metadata,
                null,
                null,
                null
        );
    }

    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public void recordTaskFailed(
            Long taskId,
            IngestionTaskStep failedStep,
            String errorCode,
            String errorMessage,
            String traceId,
            Throwable throwable
    ) {
        Map<String, Object> metadata = new LinkedHashMap<>();
        metadata.put("failedStep", failedStep == null ? null : failedStep.name());
        if (throwable != null) {
            metadata.put("exceptionClass", throwable.getClass().getSimpleName());
        }
        String displayMessage = buildFailedDisplayMessage(failedStep, errorMessage);
        record(
                taskId,
                IngestionEventType.TASK_FAILED,
                failedStep == null ? IngestionTaskStep.FAILED : failedStep,
                IngestionEventStatus.FAILED,
                "Task failed",
                displayMessage,
                null,
                metadata,
                errorCode,
                errorMessage,
                traceId
        );
    }

    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public void recordRetryRequested(Long taskId, int retryCount) {
        record(
                taskId,
                IngestionEventType.TASK_RETRY_REQUESTED,
                IngestionTaskStep.TEXT_EXTRACTED,
                IngestionEventStatus.COMPLETED,
                "Retry requested",
                "用户已请求重新处理该文档。",
                null,
                metadata("retryCount", retryCount),
                null,
                null,
                null
        );
    }

    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public void recordRetryQueued(Long taskId, int retryCount) {
        record(
                taskId,
                IngestionEventType.TASK_RETRY_QUEUED,
                IngestionTaskStep.TEXT_EXTRACTED,
                IngestionEventStatus.COMPLETED,
                "Retry queued",
                "重试任务已进入处理队列。",
                null,
                metadata("retryCount", retryCount),
                null,
                null,
                null
        );
    }

    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public void recordReindexRequested(Long taskId, Long documentId, int targetGeneration) {
        record(
                taskId,
                IngestionEventType.DOCUMENT_REINDEX_REQUESTED,
                IngestionTaskStep.TEXT_EXTRACTED,
                IngestionEventStatus.COMPLETED,
                "Reindex requested",
                "用户已请求重新建立该文档的知识库索引。",
                null,
                metadata("documentId", documentId, "targetGeneration", targetGeneration),
                null,
                null,
                null
        );
    }

    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public void recordReindexQueued(Long taskId, int targetGeneration) {
        record(
                taskId,
                IngestionEventType.DOCUMENT_REINDEX_QUEUED,
                IngestionTaskStep.TEXT_EXTRACTED,
                IngestionEventStatus.COMPLETED,
                "Reindex queued",
                "重新索引任务已进入处理队列。",
                null,
                metadata("targetGeneration", targetGeneration),
                null,
                null,
                null
        );
    }

    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public void recordReindexStarted(Long taskId, int targetGeneration) {
        record(
                taskId,
                IngestionEventType.DOCUMENT_REINDEX_STARTED,
                IngestionTaskStep.CHUNKING,
                IngestionEventStatus.STARTED,
                "Reindex started",
                "开始重新切分文档内容并生成新的知识库索引。",
                null,
                metadata("targetGeneration", targetGeneration),
                null,
                null,
                null
        );
    }

    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public void recordReindexCompleted(Long taskId, int targetGeneration, int chunkCount, long totalDurationMs) {
        record(
                taskId,
                IngestionEventType.DOCUMENT_REINDEX_COMPLETED,
                IngestionTaskStep.COMPLETED,
                IngestionEventStatus.COMPLETED,
                "Reindex completed",
                "重新索引完成，新的文档片段已可用于知识库问答。",
                totalDurationMs,
                metadata("targetGeneration", targetGeneration, "chunkCount", chunkCount),
                null,
                null,
                null
        );
    }

    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public void recordReindexFailed(
            Long taskId,
            IngestionTaskStep failedStep,
            String errorCode,
            String errorMessage,
            String traceId,
            Throwable throwable
    ) {
        Map<String, Object> metadata = new LinkedHashMap<>();
        metadata.put("failedStep", failedStep == null ? null : failedStep.name());
        if (throwable != null) {
            metadata.put("exceptionClass", throwable.getClass().getSimpleName());
        }
        String displayMessage = "重新索引失败，系统将保留旧索引继续用于问答。";
        if (errorMessage != null && !errorMessage.isBlank()) {
            displayMessage = "重新索引失败：" + errorMessage + "。系统将保留旧索引继续用于问答。";
        }
        record(
                taskId,
                IngestionEventType.DOCUMENT_REINDEX_FAILED,
                failedStep == null ? IngestionTaskStep.FAILED : failedStep,
                IngestionEventStatus.FAILED,
                "Reindex failed",
                displayMessage,
                null,
                metadata,
                errorCode,
                errorMessage,
                traceId
        );
    }

    public static String newTraceId() {
        return UUID.randomUUID().toString();
    }

    private void record(
            Long taskId,
            IngestionEventType eventType,
            IngestionTaskStep step,
            IngestionEventStatus status,
            String message,
            String displayMessage,
            Long durationMs,
            Map<String, Object> metadata,
            String errorCode,
            String errorMessage,
            String traceId
    ) {
        DocumentIngestionEventEntity event = new DocumentIngestionEventEntity();
        event.setTaskId(taskId);
        event.setEventType(eventType);
        event.setStep(step);
        event.setStatus(status);
        event.setMessage(message);
        event.setDisplayMessage(displayMessage);
        event.setDurationMs(durationMs);
        event.setMetadataJson(serializeMetadata(metadata));
        event.setErrorCode(errorCode);
        event.setErrorMessage(errorMessage);
        event.setTraceId(traceId);
        documentIngestionEventRepository.save(event);
    }

    private String buildFailedDisplayMessage(IngestionTaskStep failedStep, String errorMessage) {
        String stepHint = switch (failedStep == null ? IngestionTaskStep.FAILED : failedStep) {
            case CHUNKING -> "切分文档内容时";
            case EMBEDDING -> "生成文档向量时";
            case VECTOR_WRITING -> "写入知识库索引时";
            case TEXT_EXTRACTED -> "准备处理文档时";
            default -> "处理文档时";
        };
        if (errorMessage != null && !errorMessage.isBlank()) {
            return "处理失败：" + stepHint + "出现错误（" + errorMessage + "）。你可以稍后点击「重新处理」。";
        }
        return "处理失败：" + stepHint + "出现错误。你可以稍后点击「重新处理」。";
    }

    private Map<String, Object> metadata(Object... keyValues) {
        Map<String, Object> metadata = new LinkedHashMap<>();
        for (int index = 0; index + 1 < keyValues.length; index += 2) {
            metadata.put(String.valueOf(keyValues[index]), keyValues[index + 1]);
        }
        return metadata;
    }

    private String serializeMetadata(Map<String, Object> metadata) {
        if (metadata == null || metadata.isEmpty()) {
            return null;
        }
        try {
            return objectMapper.writeValueAsString(metadata);
        } catch (JsonProcessingException exception) {
            log.warn("Failed to serialize ingestion event metadata", exception);
            return null;
        }
    }

    public Map<String, Object> deserializeMetadata(String metadataJson) {
        if (metadataJson == null || metadataJson.isBlank()) {
            return Map.of();
        }
        try {
            return objectMapper.readValue(metadataJson, new TypeReference<>() {
            });
        } catch (JsonProcessingException exception) {
            log.warn("Failed to deserialize ingestion event metadata", exception);
            return Map.of();
        }
    }
}
