package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.document.extract.DocumentTextExtractorRegistry;
import com.tuoman.ai_task_orchestrator.document.ingestion.DocumentFileValidator;
import com.tuoman.ai_task_orchestrator.document.ingestion.DocumentIngestionEventRecorder;
import com.tuoman.ai_task_orchestrator.document.ingestion.IngestionDisplayTexts;
import com.tuoman.ai_task_orchestrator.dto.DocumentIngestionSubmitResponse;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentIngestionTaskEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStatus;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStep;
import com.tuoman.ai_task_orchestrator.mq.DocumentIngestionMessage;
import com.tuoman.ai_task_orchestrator.mq.DocumentIngestionMessagePublisher;
import com.tuoman.ai_task_orchestrator.repository.DocumentIngestionTaskRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;
import org.springframework.web.multipart.MultipartFile;

@Service
@RequiredArgsConstructor
public class DocumentIngestionService {

    private final DocumentFileValidator documentFileValidator;

    private final DocumentTextExtractorRegistry documentTextExtractorRegistry;

    private final DocumentService documentService;

    private final DocumentRepository documentRepository;

    private final DocumentIngestionTaskRepository documentIngestionTaskRepository;

    private final DocumentIngestionMessagePublisher documentIngestionMessagePublisher;

    private final DocumentIngestionEventRecorder documentIngestionEventRecorder;

    private final DocumentIngestionTaskProgressService documentIngestionTaskProgressService;

    @Transactional
    public DocumentIngestionSubmitResponse submitUpload(MultipartFile file) {
        var fileType = documentFileValidator.validate(file);
        String text = documentTextExtractorRegistry.extract(file, fileType);
        if (text == null || text.isBlank()) {
            throw BusinessException.validationError("提取的文档文本不能为空");
        }

        DocumentEntity document = documentService.createDocumentEntity(file);
        document.setSourceText(text);
        document.setStatus(DocumentStatus.UPLOADED);
        DocumentEntity savedDocument = documentRepository.save(document);

        DocumentIngestionTaskEntity task = new DocumentIngestionTaskEntity();
        task.setDocumentId(savedDocument.getId());
        task.setFilename(savedDocument.getOriginalFilename());
        task.setContentType(savedDocument.getContentType());
        task.setStatus(IngestionTaskStatus.PENDING);
        task.setStep(IngestionTaskStep.TEXT_EXTRACTED);
        task.setSourceText(text);
        DocumentIngestionTaskEntity savedTask = documentIngestionTaskRepository.save(task);

        documentIngestionEventRecorder.recordTaskCreated(
                savedTask.getId(),
                savedDocument.getOriginalFilename(),
                savedDocument.getId()
        );
        documentIngestionEventRecorder.recordTextExtracted(savedTask.getId(), savedDocument.getOriginalFilename());

        try {
            documentIngestionMessagePublisher.publish(
                    new DocumentIngestionMessage(savedTask.getId(), savedDocument.getId())
            );
            documentIngestionEventRecorder.recordTaskQueued(savedTask.getId());
        } catch (RuntimeException exception) {
            String traceId = DocumentIngestionEventRecorder.newTraceId();
            String errorCode = DocumentIngestionTaskService.resolveErrorCode(exception);
            String errorMessage = DocumentIngestionTaskService.resolveErrorMessage(exception);
            documentIngestionEventRecorder.recordTaskFailed(
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
            markDocumentFailed(savedDocument.getId(), errorMessage);
            throw BusinessException.internalError("文档已进入队列失败，请稍后重试");
        }

        IngestionTaskStatus status = savedTask.getStatus();
        return new DocumentIngestionSubmitResponse(
                savedTask.getId(),
                savedDocument.getId(),
                savedDocument.getOriginalFilename(),
                status.name(),
                IngestionDisplayTexts.displayStatus(status),
                IngestionDisplayTexts.displayMessage(status, savedTask.getStep())
        );
    }

    private void markDocumentFailed(Long documentId, String errorMessage) {
        documentRepository.findById(documentId).ifPresent(document -> {
            document.setStatus(DocumentStatus.FAILED);
            document.setErrorMessage(errorMessage);
            documentRepository.save(document);
        });
    }
}
