package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.document.ingestion.DocumentIngestionEventRecorder;
import com.tuoman.ai_task_orchestrator.document.ingestion.IngestionDisplayTexts;
import com.tuoman.ai_task_orchestrator.dto.DocumentReindexSubmitResponse;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentIngestionTaskEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStatus;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStep;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskType;
import com.tuoman.ai_task_orchestrator.mq.DocumentIngestionMessage;
import com.tuoman.ai_task_orchestrator.mq.DocumentIngestionMessagePublisher;
import com.tuoman.ai_task_orchestrator.repository.DocumentIngestionTaskRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.List;

/**
 * 单文档重新索引入口服务。
 *
 * Reindex 与普通 ingestion 的区别在于它为同一文档构建新的 generation，
 * 成功前不能覆盖旧 generation，失败时旧 READY 文档仍应保持可检索。
 *
 * 输出是一个 REINDEX 类型的 DocumentIngestionTask，实际 chunk、embedding、vector 写入
 * 仍由异步 DocumentIngestionTaskHandler 执行。
 */
@Slf4j
@Service
@RequiredArgsConstructor
public class DocumentReindexService {

    private final DocumentRepository documentRepository;

    private final DocumentIngestionTaskRepository documentIngestionTaskRepository;

    private final DocumentIngestionMessagePublisher documentIngestionMessagePublisher;

    private final DocumentIngestionEventRecorder documentIngestionEventRecorder;

    private final DocumentIngestionTaskProgressService documentIngestionTaskProgressService;

    private final DocumentService documentService;

    @Transactional
    public DocumentReindexSubmitResponse submitReindex(Long documentId) {
        DocumentEntity document = documentRepository.findById(documentId)
                .orElseThrow(BusinessException::documentNotFound);

        // TRASHED/PURGED 文档不能重建索引。
        // 否则可能把生命周期已排除的内容重新写回 active vector index。
        if (document.getLifecycleStatus() != null
                && document.getLifecycleStatus() != DocumentLifecycleStatus.ACTIVE) {
            throw BusinessException.documentDeletedCannotReindex();
        }

        if (!documentService.hasUsableSourceText(document)) {
            throw BusinessException.documentSourceTextMissing();
        }

        if (documentIngestionTaskRepository.existsByDocumentIdAndTaskTypeAndStatusIn(
                documentId,
                IngestionTaskType.REINDEX,
                List.of(IngestionTaskStatus.PENDING, IngestionTaskStatus.PROCESSING)
        )) {
            throw BusinessException.documentReindexAlreadyRunning();
        }

        // targetGeneration 单调递增，避免新旧向量共用同一个 vector identity。
        // 只有异步链路完成并校验后，Document.currentGeneration 才会切到这个版本。
        int targetGeneration = document.getCurrentGeneration() == null ? 2 : document.getCurrentGeneration() + 1;

        DocumentIngestionTaskEntity task = new DocumentIngestionTaskEntity();
        task.setDocumentId(document.getId());
        task.setFilename(document.getOriginalFilename());
        task.setContentType(document.getContentType());
        task.setTaskType(IngestionTaskType.REINDEX);
        task.setTargetGeneration(targetGeneration);
        task.setStatus(IngestionTaskStatus.PENDING);
        task.setStep(IngestionTaskStep.TEXT_EXTRACTED);
        task.setSourceText(document.getSourceText());
        DocumentIngestionTaskEntity savedTask = documentIngestionTaskRepository.save(task);

        log.info("Document reindex requested, documentId={}, taskId={}, targetGeneration={}",
                documentId, savedTask.getId(), targetGeneration);

        documentIngestionEventRecorder.recordReindexRequested(savedTask.getId(), documentId, targetGeneration);

        try {
            documentIngestionMessagePublisher.publish(
                    new DocumentIngestionMessage(savedTask.getId(), document.getId())
            );
            documentIngestionEventRecorder.recordReindexQueued(savedTask.getId(), targetGeneration);
        } catch (RuntimeException exception) {
            String traceId = DocumentIngestionEventRecorder.newTraceId();
            String errorCode = DocumentIngestionTaskService.resolveErrorCode(exception);
            String errorMessage = DocumentIngestionTaskService.resolveErrorMessage(exception);
            documentIngestionEventRecorder.recordReindexFailed(
                    savedTask.getId(),
                    IngestionTaskStep.TEXT_EXTRACTED,
                    errorCode,
                    errorMessage,
                    traceId,
                    exception
            );
            DocumentIngestionTaskService.markFailed(
                    documentIngestionTaskProgressService,
                    savedTask.getId(),
                    exception
            );
            throw BusinessException.internalError("重新索引任务进入队列失败，请稍后重试");
        }

        IngestionTaskStatus status = savedTask.getStatus();
        return new DocumentReindexSubmitResponse(
                savedTask.getId(),
                document.getId(),
                document.getOriginalFilename(),
                status.name(),
                IngestionDisplayTexts.displayStatus(status),
                IngestionDisplayTexts.reindexSubmitMessage()
        );
    }
}
