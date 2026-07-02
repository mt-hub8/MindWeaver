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

        if (document.getLifecycleStatus() == DocumentLifecycleStatus.DELETED) {
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
