package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.dto.DocumentChunkResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentDetailResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentSummaryResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentUploadResponse;
import com.tuoman.ai_task_orchestrator.document.DocumentChunkResult;
import com.tuoman.ai_task_orchestrator.document.DocumentChunker;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;
import org.springframework.web.multipart.MultipartFile;

import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.List;

@Service
@RequiredArgsConstructor
public class DocumentService {

    private final DocumentRepository documentRepository;

    private final DocumentChunkRepository documentChunkRepository;

    private final DocumentChunker documentChunker;

    @Transactional(noRollbackFor = BusinessException.class)
    public DocumentUploadResponse uploadDocument(MultipartFile file) {
        validateLegacyUploadFile(file);

        DocumentEntity savedDocument = documentRepository.save(createDocumentEntity(file));

        try {
            String content = new String(file.getBytes(), StandardCharsets.UTF_8);
            chunkAndPersist(savedDocument, content);
            return toUploadResponse(savedDocument);
        } catch (Exception e) {
            savedDocument.setStatus(DocumentStatus.FAILED);
            savedDocument.setErrorMessage(e.getMessage());
            documentRepository.save(savedDocument);
            throw BusinessException.internalError("Document processing failed");
        }
    }

    public DocumentEntity createDocumentEntity(MultipartFile file) {
        DocumentEntity document = new DocumentEntity();
        document.setOriginalFilename(file.getOriginalFilename());
        document.setContentType(file.getContentType());
        document.setFileSize(file.getSize());
        document.setStatus(DocumentStatus.UPLOADED);
        document.setChunkCount(0);
        return document;
    }

    public int chunkAndPersist(DocumentEntity document, String content) {
        List<DocumentChunkResult> chunks = documentChunker.chunk(content);
        List<DocumentChunkEntity> chunkEntities = new ArrayList<>();

        for (DocumentChunkResult chunkResult : chunks) {
            DocumentChunkEntity chunk = new DocumentChunkEntity();
            chunk.setDocumentId(document.getId());
            chunk.setChunkIndex(chunkResult.getChunkIndex());
            chunk.setContent(chunkResult.getContent());
            chunk.setContentLength(chunkResult.getContentLength());
            chunk.setChunkStrategy(chunkResult.getChunkStrategy());
            chunk.setStartOffset(chunkResult.getStartOffset());
            chunk.setEndOffset(chunkResult.getEndOffset());
            chunk.setHeadingPath(chunkResult.getHeadingPath());
            chunkEntities.add(chunk);
        }

        documentChunkRepository.saveAll(chunkEntities);

        document.setStatus(DocumentStatus.CHUNKED);
        document.setChunkCount(chunkEntities.size());
        documentRepository.save(document);
        return chunkEntities.size();
    }

    @Transactional(readOnly = true)
    public List<DocumentSummaryResponse> listDocuments() {
        return documentRepository.findAllByOrderByCreatedAtDesc()
                .stream()
                .map(this::toSummaryResponse)
                .toList();
    }

    @Transactional(readOnly = true)
    public DocumentDetailResponse getDocument(Long documentId) {
        DocumentEntity document = findDocumentOrThrow(documentId);
        return toDetailResponse(document);
    }

    @Transactional(readOnly = true)
    public List<DocumentChunkResponse> getDocumentChunks(Long documentId) {
        findDocumentOrThrow(documentId);
        return documentChunkRepository.findByDocumentIdOrderByChunkIndexAsc(documentId)
                .stream()
                .map(this::toChunkResponse)
                .toList();
    }

    private void validateLegacyUploadFile(MultipartFile file) {
        if (file == null) {
            throw BusinessException.invalidRequest("File must not be null");
        }

        String originalFilename = file.getOriginalFilename();
        if (originalFilename == null || originalFilename.isBlank()) {
            throw BusinessException.invalidRequest("Only .txt and .md files are supported");
        }

        String lowerFilename = originalFilename.toLowerCase();
        if (!lowerFilename.endsWith(".txt") && !lowerFilename.endsWith(".md")) {
            throw BusinessException.invalidRequest("Only .txt and .md files are supported");
        }
    }

    private DocumentEntity findDocumentOrThrow(Long documentId) {
        return documentRepository.findById(documentId)
                .orElseThrow(BusinessException::documentNotFound);
    }

    private DocumentSummaryResponse toSummaryResponse(DocumentEntity document) {
        return new DocumentSummaryResponse(
                document.getId(),
                document.getOriginalFilename(),
                document.getChunkCount(),
                document.getStatus().name(),
                document.getCreatedAt(),
                document.getUpdatedAt()
        );
    }

    private DocumentUploadResponse toUploadResponse(DocumentEntity document) {
        return new DocumentUploadResponse(
                document.getId(),
                document.getOriginalFilename(),
                document.getStatus().name(),
                document.getChunkCount()
        );
    }

    private DocumentDetailResponse toDetailResponse(DocumentEntity document) {
        return new DocumentDetailResponse(
                document.getId(),
                document.getOriginalFilename(),
                document.getContentType(),
                document.getFileSize(),
                document.getStatus().name(),
                document.getChunkCount(),
                document.getErrorMessage(),
                document.getCreatedAt(),
                document.getUpdatedAt()
        );
    }

    private DocumentChunkResponse toChunkResponse(DocumentChunkEntity chunk) {
        return new DocumentChunkResponse(
                chunk.getId(),
                chunk.getDocumentId(),
                chunk.getChunkIndex(),
                chunk.getContent(),
                chunk.getContentLength(),
                chunk.getChunkStrategy(),
                chunk.getStartOffset(),
                chunk.getEndOffset(),
                chunk.getHeadingPath(),
                chunk.getCreatedAt()
        );
    }
}
