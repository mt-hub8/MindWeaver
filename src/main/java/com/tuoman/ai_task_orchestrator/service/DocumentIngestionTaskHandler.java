package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.dto.DocumentEmbeddingResponse;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStatus;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStep;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;

import java.time.LocalDateTime;

@Slf4j
@Service
@RequiredArgsConstructor
public class DocumentIngestionTaskHandler {

    private final DocumentIngestionTaskService documentIngestionTaskService;

    private final DocumentIngestionTaskProgressService documentIngestionTaskProgressService;

    private final DocumentService documentService;

    private final DocumentEmbeddingService documentEmbeddingService;

    private final DocumentRepository documentRepository;

    public void process(Long taskId) {
        DocumentIngestionTaskEntitySnapshot snapshot = loadPendingTask(taskId);
        if (snapshot == null) {
            return;
        }

        try {
            documentIngestionTaskProgressService.updateTask(taskId, task -> {
                task.setStatus(IngestionTaskStatus.PROCESSING);
                task.setStep(IngestionTaskStep.CHUNKING);
            });

            DocumentEntity document = documentRepository.findById(snapshot.documentId())
                    .orElseThrow(BusinessException::documentNotFound);

            int chunkCount = documentService.chunkAndPersist(document, snapshot.sourceText());
            documentIngestionTaskProgressService.updateTask(taskId, task -> {
                task.setChunkCount(chunkCount);
                task.setStep(IngestionTaskStep.EMBEDDING);
            });

            DocumentEmbeddingResponse embeddingResponse = documentEmbeddingService.embedDocument(document.getId());
            int embeddingCount = embeddingResponse.getEmbeddedChunkCount() == null
                    ? 0
                    : embeddingResponse.getEmbeddedChunkCount();

            documentIngestionTaskProgressService.updateTask(taskId, task -> {
                task.setEmbeddingCount(embeddingCount);
                task.setStep(IngestionTaskStep.VECTOR_WRITING);
                task.setVectorWriteCount(embeddingCount);
            });

            document.setStatus(DocumentStatus.READY);
            documentRepository.save(document);

            documentIngestionTaskProgressService.updateTask(taskId, task -> {
                task.setStatus(IngestionTaskStatus.COMPLETED);
                task.setStep(IngestionTaskStep.COMPLETED);
                task.setCompletedAt(LocalDateTime.now());
            });
        } catch (Exception exception) {
            log.error("Document ingestion task failed, taskId={}", taskId, exception);
            DocumentIngestionTaskService.markFailed(
                    documentIngestionTaskProgressService,
                    taskId,
                    exception
            );
            markDocumentFailed(snapshot.documentId(), exception);
        }
    }

    private DocumentIngestionTaskEntitySnapshot loadPendingTask(Long taskId) {
        var task = documentIngestionTaskService.findTaskOrThrow(taskId);
        if (task.getStatus() != IngestionTaskStatus.PENDING) {
            log.info("Skip ingestion task because status is {}, taskId={}", task.getStatus(), taskId);
            return null;
        }
        return new DocumentIngestionTaskEntitySnapshot(task.getDocumentId(), task.getSourceText());
    }

    private void markDocumentFailed(Long documentId, Exception exception) {
        documentRepository.findById(documentId).ifPresent(document -> {
            document.setStatus(DocumentStatus.FAILED);
            document.setErrorMessage(DocumentIngestionTaskService.resolveErrorMessage(exception));
            documentRepository.save(document);
        });
    }

    private record DocumentIngestionTaskEntitySnapshot(Long documentId, String sourceText) {
    }
}
