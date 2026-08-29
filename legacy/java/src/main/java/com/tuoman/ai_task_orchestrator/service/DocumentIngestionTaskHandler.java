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
import com.tuoman.ai_task_orchestrator.vectorindex.VectorReindexIntegrationService;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.context.annotation.Lazy;
import org.springframework.stereotype.Service;

import java.time.LocalDateTime;
import java.util.Optional;

/**
 * V13.0 批量/异步摄入链路的任务执行器。
 *
 * 它消费 DocumentIngestionTaskService 创建的任务，把一次文档摄入串成：
 * chunk -> duplicate check -> embedding -> vector write -> READY。对于 reindex，
 * 它会额外接入 VectorReindexIntegrationService 管理 generation。
 *
 * 关键不变量：ingestion task 是可观测、可重试、可记录失败原因的边界；
 * reindex 失败不能破坏旧 ACTIVE generation，批量 item 失败也不能直接拖垮整个 batch。
 */
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

    private final VectorReindexIntegrationService vectorReindexIntegrationService;

    public void process(Long taskId) {
        // 任务入口只处理 PENDING task，避免 MQ 重投或手动重试造成同一任务重复执行。
        // INGEST 和 REINDEX 共用后半段 embedding/vector 写入，但 generation 语义不同。
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
        // 普通 ingestion：文本已经被上传/解析并保存在 task snapshot 中。
        // 这里按固定顺序执行 chunk、文本级重复检测、embedding、vector write。
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
        // reindex 与普通 ingestion 的区别在于目标 generation。
        // 新 generation 构建期间不能参与检索；只有后续向量写入和激活完成后才替换旧 generation。
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

            vectorReindexIntegrationService.onReindexStarted(document.getId(), targetGeneration);

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
        // embedding 阶段会通过 EmbeddingCacheService 复用相同 chunk 内容的向量结果；
        // vector write 阶段由 IdempotentVectorUpsertService 确保重试不会产生重复 vector。
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
            // reindex 成功后才激活新 generation 并退休旧 generation。
            // 如果这里之前失败，旧 ACTIVE generation 必须保持不变。
            documentService.completeReindexGeneration(documentId, targetGeneration, embeddingCount);
            vectorReindexIntegrationService.onReindexCompleted(documentId, targetGeneration);
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
        // 摄入失败同时写 ingestion event、task status 和 document status。
        // 这样 UI、通知和后续重试都能定位失败阶段与 traceId。
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
        // reindex 失败必须清理 target generation，并标记 vector generation FAILED。
        // 不允许把旧 ACTIVE generation 一并删除，否则会出现知识库短暂不可检索。
        log.error("Document reindex task failed, taskId={}, documentId={}", taskId, documentId, exception);
        if (targetGeneration != null) {
            documentService.cleanupFailedReindexGeneration(documentId, targetGeneration);
            vectorReindexIntegrationService.onReindexFailed(documentId, targetGeneration, exception.getMessage());
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
        // 文本级重复发生在 chunk 后、embedding 前。
        // 这样可以避免为重复文本生成 embedding/vector，同时让 batch item 以 SKIPPED_DUPLICATE_TEXT 收敛。
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
