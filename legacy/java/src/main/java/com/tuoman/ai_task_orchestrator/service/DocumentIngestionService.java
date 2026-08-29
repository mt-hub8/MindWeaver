package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.document.extract.DocumentTextExtractorRegistry;
import com.tuoman.ai_task_orchestrator.document.ingestion.DocumentFileValidator;
import com.tuoman.ai_task_orchestrator.document.ingestion.DocumentIngestionEventRecorder;
import com.tuoman.ai_task_orchestrator.document.ingestion.FileHashService;
import com.tuoman.ai_task_orchestrator.document.ingestion.IngestionDisplayTexts;
import com.tuoman.ai_task_orchestrator.dto.DocumentIngestionSubmitResponse;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentIngestionTaskEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStatus;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStep;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskType;
import com.tuoman.ai_task_orchestrator.mq.DocumentIngestionMessage;
import com.tuoman.ai_task_orchestrator.mq.DocumentIngestionMessagePublisher;
import com.tuoman.ai_task_orchestrator.repository.DocumentIngestionTaskRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;
import org.springframework.web.multipart.MultipartFile;

import java.io.IOException;

/**
 * 文档异步摄入入口服务。
 *
 * V13 的 batch ingestion 之后，普通上传和批量导入都会先创建 DocumentEntity
 * 与 DocumentIngestionTaskEntity，再把耗时的 chunk、embedding、vector write 交给 MQ worker。
 *
 * 关键不变量：这里创建的是可追踪任务，不是最终可检索文档；只有后续异步链路完成后，
 * DocumentStatus 才能从 UPLOADED/CHUNKED 进入 READY。
 */
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

    private final FileHashService fileHashService;

    @Transactional
    public DocumentIngestionSubmitResponse submitUpload(MultipartFile file) {
        // 普通上传入口只同步完成文件校验和文本提取。
        // 后续结构化切分、embedding 和 vector 写入必须异步执行，避免上传请求被模型/向量库耗时阻塞。
        var fileType = documentFileValidator.validate(file);
        String text = documentTextExtractorRegistry.extract(file, fileType);
        if (text == null || text.isBlank()) {
            throw BusinessException.validationError("提取的文档文本不能为空");
        }
        try {
            byte[] bytes = file.getBytes();
            return submitIngestion(
                    file.getOriginalFilename(),
                    file.getContentType(),
                    file.getSize(),
                    bytes,
                    text,
                    null
            );
        } catch (IOException exception) {
            throw BusinessException.internalError("读取上传文件失败");
        }
    }

    @Transactional
    public DocumentIngestionSubmitResponse submitIngestion(
            String originalFilename,
            String contentType,
            long fileSize,
            byte[] fileBytes,
            String text,
            Long batchItemId
    ) {
        if (text == null || text.isBlank()) {
            throw BusinessException.validationError("提取的文档文本不能为空");
        }

        // fileHash 用于文件级去重，textHash 用于内容级去重。
        // 二者都属于摄入诊断信息，不应参与 vector identity；否则 retryCount 或批次差异会制造重复向量。
        String fileHash = fileHashService.hashBytes(fileBytes);
        String textHash = fileHashService.hashText(text);

        // 先持久化 DocumentEntity，再创建 ingestion task。
        // 此时文档仅表示“已接收”，还不能进入检索 final context。
        DocumentEntity document = documentService.createDocumentEntityFromMeta(
                originalFilename,
                contentType,
                fileSize
        );
        document.setSourceText(text);
        document.setFileHash(fileHash);
        document.setTextHash(textHash);
        document.setCurrentGeneration(1);
        document.setReindexCount(0);
        document.setStatus(DocumentStatus.UPLOADED);
        DocumentEntity savedDocument = documentRepository.save(document);

        DocumentIngestionTaskEntity task = new DocumentIngestionTaskEntity();
        task.setDocumentId(savedDocument.getId());
        task.setFilename(savedDocument.getOriginalFilename());
        task.setContentType(savedDocument.getContentType());
        task.setStatus(IngestionTaskStatus.PENDING);
        task.setStep(IngestionTaskStep.TEXT_EXTRACTED);
        task.setTaskType(IngestionTaskType.INGEST);
        task.setSourceText(text);
        task.setBatchItemId(batchItemId);
        DocumentIngestionTaskEntity savedTask = documentIngestionTaskRepository.save(task);

        // 事件记录让前端和排障流程能看到任务从 created -> queued -> processing -> terminal 的完整轨迹。
        // 即使后续 MQ publish 失败，也能把失败归因停在 TEXT_EXTRACTED 阶段。
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
