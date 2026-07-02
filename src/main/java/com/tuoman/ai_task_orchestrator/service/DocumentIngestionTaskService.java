package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.common.error.ErrorCode;
import com.tuoman.ai_task_orchestrator.document.lifecycle.DocumentLifecycleDisplayTexts;
import com.tuoman.ai_task_orchestrator.document.ingestion.DocumentIngestionEventRecorder;
import com.tuoman.ai_task_orchestrator.document.ingestion.DocumentIngestionProperties;
import com.tuoman.ai_task_orchestrator.document.ingestion.IngestionDisplayTexts;
import com.tuoman.ai_task_orchestrator.dto.DocumentIngestionTaskResponse;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentIngestionEventEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentIngestionTaskEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStatus;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStep;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskType;
import com.tuoman.ai_task_orchestrator.mq.DocumentIngestionMessage;
import com.tuoman.ai_task_orchestrator.mq.DocumentIngestionMessagePublisher;
import com.tuoman.ai_task_orchestrator.repository.DocumentIngestionEventRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentIngestionTaskRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.LocalDateTime;
import java.util.Comparator;
import java.util.List;

@Service
@RequiredArgsConstructor
public class DocumentIngestionTaskService {

    private final DocumentIngestionTaskRepository documentIngestionTaskRepository;

    private final DocumentIngestionEventRepository documentIngestionEventRepository;

    private final DocumentIngestionMessagePublisher documentIngestionMessagePublisher;

    private final DocumentIngestionProperties documentIngestionProperties;

    private final DocumentService documentService;

    private final DocumentIngestionEventRecorder documentIngestionEventRecorder;

    private final DocumentRepository documentRepository;

    @Transactional(readOnly = true)
    public DocumentIngestionTaskResponse getTask(Long taskId) {
        return toResponse(findTaskOrThrow(taskId));
    }

    @Transactional(readOnly = true)
    public List<DocumentIngestionTaskResponse> listRecentTasks() {
        return documentIngestionTaskRepository.findAll().stream()
                .sorted(Comparator.comparing(DocumentIngestionTaskEntity::getCreatedAt).reversed())
                .limit(documentIngestionProperties.getRecentTaskLimit())
                .map(this::toResponse)
                .toList();
    }

    @Transactional
    public DocumentIngestionTaskResponse retryTask(Long taskId) {
        DocumentIngestionTaskEntity task = findTaskOrThrow(taskId);
        if (task.getStatus() != IngestionTaskStatus.FAILED) {
            throw BusinessException.ingestionRetryNotAllowed("只有失败状态的任务可以重新处理");
        }
        if (task.getRetryCount() >= documentIngestionProperties.getMaxRetryCount()) {
            throw BusinessException.ingestionMaxRetryExceeded();
        }

        task.setRetryCount(task.getRetryCount() + 1);
        task.setStatus(IngestionTaskStatus.PENDING);
        task.setStep(IngestionTaskStep.TEXT_EXTRACTED);
        task.setChunkCount(0);
        task.setEmbeddingCount(0);
        task.setVectorWriteCount(0);
        task.setErrorCode(null);
        task.setErrorMessage(null);
        task.setCompletedAt(null);
        documentIngestionTaskRepository.save(task);

        documentService.clearChunksForRetry(task.getDocumentId());

        documentIngestionEventRecorder.recordRetryRequested(taskId, task.getRetryCount());

        try {
            documentIngestionMessagePublisher.publish(
                    new DocumentIngestionMessage(task.getId(), task.getDocumentId())
            );
            documentIngestionEventRecorder.recordRetryQueued(taskId, task.getRetryCount());
        } catch (RuntimeException exception) {
            String traceId = DocumentIngestionEventRecorder.newTraceId();
            documentIngestionEventRecorder.recordTaskFailed(
                    taskId,
                    IngestionTaskStep.TEXT_EXTRACTED,
                    resolveErrorCode(exception),
                    resolveErrorMessage(exception),
                    traceId,
                    exception
            );
            throw BusinessException.internalError("重试任务进入队列失败，请稍后再试");
        }

        return toResponse(task);
    }

    public DocumentIngestionTaskEntity findTaskOrThrow(Long taskId) {
        return documentIngestionTaskRepository.findById(taskId)
                .orElseThrow(BusinessException::ingestionTaskNotFound);
    }

    public DocumentIngestionTaskResponse toResponse(DocumentIngestionTaskEntity task) {
        IngestionTaskStatus status = task.getStatus();
        IngestionTaskStep step = task.getStep();
        DocumentIngestionEventEntity latestEvent = documentIngestionEventRepository
                .findTopByTaskIdOrderByCreatedAtDesc(task.getId())
                .orElse(null);
        DocumentLifecycleFields lifecycleFields = resolveLifecycleFields(task.getDocumentId());
        IngestionTaskType taskType = task.getTaskType() == null ? IngestionTaskType.INGEST : task.getTaskType();
        ReindexFields reindexFields = resolveReindexFields(task.getDocumentId(), lifecycleFields);
        return new DocumentIngestionTaskResponse(
                task.getId(),
                task.getDocumentId(),
                task.getFilename(),
                status.name(),
                IngestionDisplayTexts.displayStatus(status),
                step.name(),
                IngestionDisplayTexts.displayStep(step),
                resolveDisplayMessage(task),
                task.getChunkCount(),
                task.getEmbeddingCount(),
                task.getVectorWriteCount(),
                task.getErrorCode(),
                task.getErrorMessage(),
                task.getRetryCount(),
                task.getCreatedAt(),
                task.getUpdatedAt(),
                task.getCompletedAt(),
                latestEvent == null ? null : latestEvent.getDisplayMessage(),
                latestEvent == null ? null : latestEvent.getCreatedAt(),
                lifecycleFields.lifecycleStatus(),
                lifecycleFields.displayStatus(),
                lifecycleFields.deletedAt(),
                lifecycleFields.canDelete(),
                lifecycleFields.canAsk(),
                taskType.name(),
                IngestionDisplayTexts.displayTaskType(taskType),
                reindexFields.canReindex(),
                reindexFields.disabledReason()
        );
    }

    private DocumentLifecycleFields resolveLifecycleFields(Long documentId) {
        if (documentId == null) {
            return DocumentLifecycleFields.defaults();
        }
        return documentRepository.findById(documentId)
                .map(this::toLifecycleFields)
                .orElse(DocumentLifecycleFields.defaults());
    }

    private DocumentLifecycleFields toLifecycleFields(DocumentEntity document) {
        DocumentLifecycleStatus lifecycleStatus = document.getLifecycleStatus() == null
                ? DocumentLifecycleStatus.ACTIVE
                : document.getLifecycleStatus();
        boolean active = lifecycleStatus == DocumentLifecycleStatus.ACTIVE;
        return new DocumentLifecycleFields(
                lifecycleStatus.name(),
                DocumentLifecycleDisplayTexts.displayStatus(lifecycleStatus),
                document.getDeletedAt(),
                active,
                active && document.getStatus() == DocumentStatus.READY
        );
    }

    private record DocumentLifecycleFields(
            String lifecycleStatus,
            String displayStatus,
            LocalDateTime deletedAt,
            Boolean canDelete,
            Boolean canAsk
    ) {
        static DocumentLifecycleFields defaults() {
            return new DocumentLifecycleFields(
                    DocumentLifecycleStatus.ACTIVE.name(),
                    DocumentLifecycleDisplayTexts.displayStatus(DocumentLifecycleStatus.ACTIVE),
                    null,
                    true,
                    false
            );
        }
    }

    private String resolveDisplayMessage(DocumentIngestionTaskEntity task) {
        if (task.getStatus() == IngestionTaskStatus.FAILED && task.getErrorMessage() != null) {
            if (task.getTaskType() == IngestionTaskType.REINDEX) {
                return "重新索引失败：" + task.getErrorMessage();
            }
            return "处理失败：" + task.getErrorMessage();
        }
        if (task.getTaskType() == IngestionTaskType.REINDEX) {
            return IngestionDisplayTexts.reindexDisplayMessage(task.getStatus(), task.getStep());
        }
        return IngestionDisplayTexts.displayMessage(task.getStatus(), task.getStep());
    }

    private ReindexFields resolveReindexFields(Long documentId, DocumentLifecycleFields lifecycleFields) {
        if (documentId == null) {
            return ReindexFields.disabled("文档不存在");
        }
        if (!Boolean.TRUE.equals(lifecycleFields.canDelete())) {
            return ReindexFields.disabled("当前文档已删除，不能重新索引");
        }
        return documentRepository.findById(documentId)
                .map(document -> {
                    if (!documentService.hasUsableSourceText(document)) {
                        return ReindexFields.disabled("当前文档缺少原始文本，无法重新索引");
                    }
                    return ReindexFields.enabled();
                })
                .orElse(ReindexFields.disabled("文档不存在"));
    }

    private record ReindexFields(Boolean canReindex, String disabledReason) {
        static ReindexFields enabled() {
            return new ReindexFields(true, null);
        }

        static ReindexFields disabled(String reason) {
            return new ReindexFields(false, reason);
        }
    }

    public static String resolveErrorCode(Throwable throwable) {
        if (throwable instanceof BusinessException businessException) {
            ErrorCode errorCode = businessException.getErrorCode();
            return errorCode == null ? ErrorCode.INTERNAL_ERROR.name() : errorCode.name();
        }
        return ErrorCode.INTERNAL_ERROR.name();
    }

    public static String resolveErrorMessage(Throwable throwable) {
        if (throwable instanceof BusinessException businessException) {
            return businessException.getMessage();
        }
        return throwable.getMessage() == null ? "文档处理失败" : throwable.getMessage();
    }

    public static void markFailed(
            DocumentIngestionTaskProgressService progressService,
            Long taskId,
            Throwable throwable
    ) {
        progressService.updateTask(taskId, task -> {
            task.setStatus(IngestionTaskStatus.FAILED);
            task.setStep(IngestionTaskStep.FAILED);
            task.setErrorCode(resolveErrorCode(throwable));
            task.setErrorMessage(resolveErrorMessage(throwable));
            task.setCompletedAt(LocalDateTime.now());
        });
    }
}
