package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.batch.DuplicateDetectionService;
import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.document.ingestion.DocumentIngestionEventRecorder;
import com.tuoman.ai_task_orchestrator.dto.DocumentEmbeddingResponse;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStatus;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStep;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskType;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.context.annotation.Lazy;
import org.springframework.stereotype.Service;

import java.time.LocalDateTime;
import java.util.Optional;

@Slf4j
@Service
@RequiredArgsConstructor
public class DocumentIngestionTaskHandler {

    private final DocumentIngestionTaskService documentIngestionTaskService;

    private final DocumentIngestionTaskProgressService documentIngestionTaskProgressService;

    private final DocumentService documentService;

    private final DocumentEmbeddingService documentEmbeddingService;

    private final DocumentRepository documentRepository;

    private final DocumentIngestionEventRecorder documentIngestionEventRecorder;

    private final DuplicateDetectionService duplicateDetectionService;

    @Lazy
    private final UploadBatchService uploadBatchService;

    public void process(Long taskId) {
        DocumentIngestionTaskEntitySnapshot snapshot = loadPendingTask(taskId);
        if (snapshot == null) {
            return;
        }

        if (snapshot.taskType() == IngestionTaskType.REINDEX) {
            processReindex(taskId, snapshot);
            return;
        }
        processIngest(taskId, snapshot);
    }

    private void processIngest(Long taskId, DocumentIngestionTaskEntitySnapshot snapshot) {
        long taskStartedAt = System.currentTimeMillis();
        IngestionTaskStep currentStep = IngestionTaskStep.CHUNKING;

        try {
            documentIngestionEventRecorder.recordTaskStarted(taskId);
            uploadBatchService.onIngestionTaskStarted(taskId);
            documentIngestionTaskProgressService.updateTask(taskId, task -> {
                task.setStatus(IngestionTaskStatus.PROCESSING);
                task.setStep(IngestionTaskStep.CHUNKING);
            });

            DocumentEntity document = documentRepository.findById(snapshot.documentId())
                    .orElseThrow(BusinessException::documentNotFound);

            currentStep = IngestionTaskStep.CHUNKING;
            documentIngestionEventRecorder.recordChunkingStarted(taskId);
            long chunkingStartedAt = System.currentTimeMillis();
            int chunkCount = documentService.chunkAndPersist(document, snapshot.sourceText());
            documentIngestionEventRecorder.recordChunkingCompleted(
                    taskId,
                    chunkCount,
                    System.currentTimeMillis() - chunkingStartedAt
            );

            documentIngestionTaskProgressService.updateTask(taskId, task -> task.setChunkCount(chunkCount));

            if (shouldSkipTextDuplicate(taskId, snapshot, document)) {
                return;
            }

            completeEmbeddingAndVectorStages(taskId, document.getId(), currentStep, taskStartedAt, false, null);
            uploadBatchService.onIngestionTaskCompleted(taskId);
        } catch (Exception exception) {
            handleIngestFailure(taskId, snapshot.documentId(), snapshot.batchItemId(), currentStep, exception);
        }
    }

    private void processReindex(Long taskId, DocumentIngestionTaskEntitySnapshot snapshot) {
        long taskStartedAt = System.currentTimeMillis();
        IngestionTaskStep currentStep = IngestionTaskStep.CHUNKING;
        final Integer targetGeneration = snapshot.targetGeneration();

        try {
            DocumentEntity document = documentRepository.findById(snapshot.documentId())
                    .orElseThrow(BusinessException::documentNotFound);
            if (document.getLifecycleStatus() != null
                    && document.getLifecycleStatus() != DocumentLifecycleStatus.ACTIVE) {
                throw BusinessException.documentDeletedCannotReindex();
            }
            if (!documentService.hasUsableSourceText(document)) {
                throw BusinessException.documentSourceTextMissing();
            }
            if (targetGeneration == null) {
                throw BusinessException.validationError("重新索引任务缺少目标版本号");
            }

            documentIngestionEventRecorder.recordReindexStarted(taskId, targetGeneration);
            documentIngestionTaskProgressService.updateTask(taskId, task -> {
                task.setStatus(IngestionTaskStatus.PROCESSING);
                task.setStep(IngestionTaskStep.CHUNKING);
            });

            currentStep = IngestionTaskStep.CHUNKING;
            documentIngestionEventRecorder.recordChunkingStarted(taskId);
            long chunkingStartedAt = System.currentTimeMillis();
            int chunkCount = documentService.chunkAndPersistForGeneration(
                    document,
                    snapshot.sourceText(),
                    targetGeneration
            );
            documentIngestionEventRecorder.recordChunkingCompleted(
                    taskId,
                    chunkCount,
                    System.currentTimeMillis() - chunkingStartedAt
            );
            documentIngestionTaskProgressService.updateTask(taskId, task -> task.setChunkCount(chunkCount));

            completeEmbeddingAndVectorStages(taskId, document.getId(), currentStep, taskStartedAt, true, targetGeneration);
        } catch (Exception exception) {
            handleReindexFailure(taskId, snapshot.documentId(), targetGeneration, currentStep, exception);
        }
    }

    private void completeEmbeddingAndVectorStages(
            Long taskId,
            Long documentId,
            IngestionTaskStep currentStep,
            long taskStartedAt,
            boolean reindex,
            Integer targetGeneration
    ) throws Exception {
        documentIngestionTaskProgressService.updateTask(taskId, task -> task.setStep(IngestionTaskStep.EMBEDDING));
        documentIngestionEventRecorder.recordEmbeddingStarted(taskId);
        long embeddingStartedAt = System.currentTimeMillis();

        DocumentEmbeddingResponse embeddingResponse = reindex
                ? documentEmbeddingService.embedDocumentGeneration(documentId, targetGeneration, false)
                : documentEmbeddingService.embedDocument(documentId);

        int embeddingCount = embeddingResponse.getEmbeddedChunkCount() == null
                ? 0
                : embeddingResponse.getEmbeddedChunkCount();
        documentIngestionEventRecorder.recordEmbeddingCompleted(
                taskId,
                embeddingCount,
                System.currentTimeMillis() - embeddingStartedAt
        );

        documentIngestionEventRecorder.recordVectorWriteStarted(taskId);
        long vectorWriteStartedAt = System.currentTimeMillis();
        documentIngestionEventRecorder.recordVectorWriteCompleted(
                taskId,
                embeddingCount,
                System.currentTimeMillis() - vectorWriteStartedAt
        );

        documentIngestionTaskProgressService.updateTask(taskId, task -> {
            task.setEmbeddingCount(embeddingCount);
            task.setStep(IngestionTaskStep.VECTOR_WRITING);
            task.setVectorWriteCount(embeddingCount);
        });

        if (reindex) {
            documentService.completeReindexGeneration(documentId, targetGeneration, embeddingCount);
            documentIngestionTaskProgressService.updateTask(taskId, task -> {
                task.setStatus(IngestionTaskStatus.COMPLETED);
                task.setStep(IngestionTaskStep.COMPLETED);
                task.setCompletedAt(LocalDateTime.now());
            });
            documentIngestionEventRecorder.recordReindexCompleted(
                    taskId,
                    targetGeneration,
                    embeddingCount,
                    System.currentTimeMillis() - taskStartedAt
            );
            return;
        }

        DocumentEntity document = documentRepository.findById(documentId)
                .orElseThrow(BusinessException::documentNotFound);
        document.setStatus(DocumentStatus.READY);
        documentRepository.save(document);

        documentIngestionTaskProgressService.updateTask(taskId, task -> {
            task.setStatus(IngestionTaskStatus.COMPLETED);
            task.setStep(IngestionTaskStep.COMPLETED);
            task.setCompletedAt(LocalDateTime.now());
        });
        documentIngestionEventRecorder.recordTaskCompleted(
                taskId,
                System.currentTimeMillis() - taskStartedAt
        );
    }

    private void handleIngestFailure(
            Long taskId,
            Long documentId,
            Long batchItemId,
            IngestionTaskStep currentStep,
            Exception exception
    ) {
        log.error("Document ingestion task failed, taskId={}", taskId, exception);
        String traceId = DocumentIngestionEventRecorder.newTraceId();
        String errorCode = DocumentIngestionTaskService.resolveErrorCode(exception);
        String errorMessage = DocumentIngestionTaskService.resolveErrorMessage(exception);
        documentIngestionEventRecorder.recordTaskFailed(
                taskId,
                currentStep,
                errorCode,
                errorMessage,
                traceId,
                exception
        );
        DocumentIngestionTaskService.markFailed(
                documentIngestionTaskProgressService,
                taskId,
                exception
        );
        markDocumentFailed(documentId, exception);
        if (batchItemId != null) {
            uploadBatchService.onIngestionTaskFailed(taskId, errorCode, errorMessage);
        }
    }

    private void handleReindexFailure(
            Long taskId,
            Long documentId,
            Integer targetGeneration,
            IngestionTaskStep currentStep,
            Exception exception
    ) {
        log.error("Document reindex task failed, taskId={}, documentId={}", taskId, documentId, exception);
        if (targetGeneration != null) {
            documentService.cleanupFailedReindexGeneration(documentId, targetGeneration);
        }
        String traceId = DocumentIngestionEventRecorder.newTraceId();
        String errorCode = DocumentIngestionTaskService.resolveErrorCode(exception);
        String errorMessage = DocumentIngestionTaskService.resolveErrorMessage(exception);
        documentIngestionEventRecorder.recordReindexFailed(
                taskId,
                currentStep,
                errorCode,
                errorMessage,
                traceId,
                exception
        );
        DocumentIngestionTaskService.markFailed(
                documentIngestionTaskProgressService,
                taskId,
                exception
        );
    }

    private DocumentIngestionTaskEntitySnapshot loadPendingTask(Long taskId) {
        var task = documentIngestionTaskService.findTaskOrThrow(taskId);
        if (task.getStatus() != IngestionTaskStatus.PENDING) {
            log.info("Skip ingestion task because status is {}, taskId={}", task.getStatus(), taskId);
            return null;
        }
        IngestionTaskType taskType = task.getTaskType() == null ? IngestionTaskType.INGEST : task.getTaskType();
        return new DocumentIngestionTaskEntitySnapshot(
                task.getDocumentId(),
                task.getSourceText(),
                taskType,
                task.getTargetGeneration(),
                task.getBatchItemId()
        );
    }

    private boolean shouldSkipTextDuplicate(
            Long taskId,
            DocumentIngestionTaskEntitySnapshot snapshot,
            DocumentEntity document
    ) {
        if (snapshot.batchItemId() == null) {
            return false;
        }
        Optional<Long> duplicateDocumentId = duplicateDetectionService.findActiveTextDuplicate(
                document.getTextHash(),
                document.getId()
        );
        if (duplicateDocumentId.isEmpty()) {
            return false;
        }

        document.setStatus(DocumentStatus.FAILED);
        document.setErrorMessage("检测到相同文本内容，已跳过重复 embedding。");
        documentRepository.save(document);

        documentIngestionTaskProgressService.updateTask(taskId, task -> {
            task.setStatus(IngestionTaskStatus.COMPLETED);
            task.setStep(IngestionTaskStep.COMPLETED);
            task.setCompletedAt(LocalDateTime.now());
        });
        documentIngestionEventRecorder.recordTaskCompleted(taskId, 0L);
        uploadBatchService.markTextDuplicateSkipped(snapshot.batchItemId(), duplicateDocumentId.get());
        return true;
    }

    private void markDocumentFailed(Long documentId, Exception exception) {
        documentRepository.findById(documentId).ifPresent(document -> {
            document.setStatus(DocumentStatus.FAILED);
            document.setErrorMessage(DocumentIngestionTaskService.resolveErrorMessage(exception));
            documentRepository.save(document);
        });
    }

    private record DocumentIngestionTaskEntitySnapshot(
            Long documentId,
            String sourceText,
            IngestionTaskType taskType,
            Integer targetGeneration,
            Long batchItemId
    ) {
    }
}
