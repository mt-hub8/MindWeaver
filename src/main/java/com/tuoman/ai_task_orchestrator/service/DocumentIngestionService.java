package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.document.extract.DocumentFileType;
import com.tuoman.ai_task_orchestrator.document.extract.DocumentTextExtractorRegistry;
import com.tuoman.ai_task_orchestrator.document.ingestion.DocumentFileValidator;
import com.tuoman.ai_task_orchestrator.dto.DocumentEmbeddingResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentIngestionResponse;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
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

    private final DocumentEmbeddingService documentEmbeddingService;

    private final DocumentRepository documentRepository;

    @Transactional(noRollbackFor = BusinessException.class)
    public DocumentIngestionResponse ingest(MultipartFile file) {
        DocumentFileType fileType = documentFileValidator.validate(file);
        String text = documentTextExtractorRegistry.extract(file, fileType);
        if (text == null || text.isBlank()) {
            throw BusinessException.validationError("Extracted document text must not be empty");
        }

        DocumentEntity document = documentService.createDocumentEntity(file);
        DocumentEntity savedDocument = documentRepository.save(document);

        try {
            int chunkCount = documentService.chunkAndPersist(savedDocument, text);
            DocumentEmbeddingResponse embeddingResponse = documentEmbeddingService.embedDocument(savedDocument.getId());
            int embeddingCount = embeddingResponse.getEmbeddedChunkCount() == null ? 0 : embeddingResponse.getEmbeddedChunkCount();

            savedDocument.setStatus(DocumentStatus.READY);
            documentRepository.save(savedDocument);

            return new DocumentIngestionResponse(
                    savedDocument.getId(),
                    savedDocument.getOriginalFilename(),
                    savedDocument.getStatus().name(),
                    chunkCount,
                    embeddingCount,
                    embeddingCount
            );
        } catch (BusinessException exception) {
            markFailed(savedDocument, exception.getMessage());
            throw exception;
        } catch (RuntimeException exception) {
            markFailed(savedDocument, exception.getMessage());
            throw BusinessException.internalError(
                    exception.getMessage() == null ? "Document ingestion failed" : exception.getMessage()
            );
        }
    }

    private void markFailed(DocumentEntity document, String errorMessage) {
        document.setStatus(DocumentStatus.FAILED);
        document.setErrorMessage(errorMessage);
        documentRepository.save(document);
    }
}
