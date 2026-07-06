package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.document.lifecycle.DocumentLifecycleDisplayTexts;
import com.tuoman.ai_task_orchestrator.dto.CollectionMembershipResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentChunkResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentDeleteResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentDetailResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentSummaryResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentUploadResponse;
import com.tuoman.ai_task_orchestrator.document.StructuredChunkResult;
import com.tuoman.ai_task_orchestrator.document.StructuredChunkingService;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.ChunkStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;
import org.springframework.web.multipart.MultipartFile;

import java.nio.charset.StandardCharsets;
import java.time.LocalDateTime;
import java.util.ArrayList;
import java.util.List;

@Slf4j
@Service
@RequiredArgsConstructor
public class DocumentService {

    private final DocumentRepository documentRepository;

    private final DocumentChunkRepository documentChunkRepository;

    private final StructuredChunkingService structuredChunkingService;

    private final ChunkMetadataService chunkMetadataService;

    private final CollectionService collectionService;

    private final DocumentTrashService documentTrashService;

    @Transactional(noRollbackFor = BusinessException.class)
    public DocumentUploadResponse uploadDocument(MultipartFile file) {
        validateLegacyUploadFile(file);

        DocumentEntity savedDocument = documentRepository.save(createDocumentEntity(file));

        try {
            String content = new String(file.getBytes(), StandardCharsets.UTF_8);
            savedDocument.setSourceText(content);
            documentRepository.save(savedDocument);
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
        return createDocumentEntityFromMeta(file.getOriginalFilename(), file.getContentType(), file.getSize());
    }

    public DocumentEntity createDocumentEntityFromMeta(String originalFilename, String contentType, long fileSize) {
        DocumentEntity document = new DocumentEntity();
        document.setOriginalFilename(originalFilename);
        document.setContentType(contentType);
        document.setFileSize(fileSize);
        document.setStatus(DocumentStatus.UPLOADED);
        document.setLifecycleStatus(DocumentLifecycleStatus.ACTIVE);
        document.setCurrentGeneration(1);
        document.setReindexCount(0);
        document.setChunkCount(0);
        return document;
    }

    public int chunkAndPersist(DocumentEntity document, String content) {
        int generation = document.getCurrentGeneration() == null ? 1 : document.getCurrentGeneration();
        return chunkAndPersistForGeneration(document, content, generation);
    }

    public int chunkAndPersistForGeneration(DocumentEntity document, String content, int generation) {
        chunkMetadataService.applyDocumentMetadata(document);
        List<StructuredChunkResult> chunks = structuredChunkingService.chunk(content);
        List<DocumentChunkEntity> chunkEntities = new ArrayList<>();
        List<Integer> parentChunkIndices = new ArrayList<>();

        for (StructuredChunkResult chunkResult : chunks) {
            DocumentChunkEntity chunk = new DocumentChunkEntity();
            chunk.setDocumentId(document.getId());
            chunk.setChunkIndex(chunkResult.getChunkIndex());
            chunk.setContent(chunkResult.getContent());
            chunk.setContentLength(chunkResult.getContentLength());
            chunk.setChunkStrategy(chunkResult.getChunkStrategy());
            chunk.setStartOffset(chunkResult.getStartOffset());
            chunk.setEndOffset(chunkResult.getEndOffset());
            chunk.setHeadingPath(chunkResult.getHeadingPath());
            chunk.setSectionPath(chunkResult.getSectionPath());
            chunk.setSectionTitle(chunkResult.getSectionTitle());
            chunk.setHeadingLevel(chunkResult.getHeadingLevel());
            chunk.setChunkType(chunkResult.getChunkType());
            chunk.setContentHash(chunkResult.getContentHash());
            chunk.setNormalizedContentHash(chunkResult.getNormalizedContentHash());
            chunk.setTokenCount(chunkResult.getTokenCount());
            chunk.setCharCount(chunkResult.getContentLength());
            chunk.setLanguage(chunkResult.getLanguage());
            chunk.setChunkStatus(ChunkStatus.ACTIVE);
            chunk.setGeneration(generation);
            chunkMetadataService.applyChunkMetadata(document, chunk);
            chunkEntities.add(chunk);
            parentChunkIndices.add(chunkResult.getParentChunkIndex());
        }

        documentChunkRepository.saveAll(chunkEntities);
        chunkMetadataService.linkChunkRelations(chunkEntities, parentChunkIndices);
        documentChunkRepository.saveAll(chunkEntities);

        document.setStatus(DocumentStatus.CHUNKED);
        document.setChunkCount(countActiveChunksAtGeneration(document.getId(), generation));
        documentRepository.save(document);
        return chunkEntities.size();
    }

    @Transactional
    public void completeReindexGeneration(Long documentId, int newGeneration, int chunkCount) {
        DocumentEntity document = findDocumentOrThrow(documentId);
        documentChunkRepository.supersedeChunksBeforeGeneration(documentId, newGeneration);
        document.setCurrentGeneration(newGeneration);
        document.setReindexCount((document.getReindexCount() == null ? 0 : document.getReindexCount()) + 1);
        document.setLastReindexedAt(LocalDateTime.now());
        document.setChunkCount(chunkCount);
        document.setStatus(DocumentStatus.READY);
        document.setErrorMessage(null);
        documentRepository.save(document);
    }

    @Transactional
    public void cleanupFailedReindexGeneration(Long documentId, int generation) {
        documentChunkRepository.deleteByDocumentIdAndGeneration(documentId, generation);
        DocumentEntity document = findDocumentOrThrow(documentId);
        int currentGeneration = document.getCurrentGeneration() == null ? 1 : document.getCurrentGeneration();
        document.setChunkCount(countActiveChunksAtGeneration(documentId, currentGeneration));
        if (document.getStatus() != DocumentStatus.READY) {
            document.setStatus(DocumentStatus.READY);
        }
        documentRepository.save(document);
    }

    public boolean hasUsableSourceText(DocumentEntity document) {
        return document != null
                && document.getSourceText() != null
                && !document.getSourceText().isBlank();
    }

    public int countActiveChunksAtGeneration(Long documentId, int generation) {
        return documentChunkRepository.countByDocumentIdAndChunkStatusAndGeneration(
                documentId,
                ChunkStatus.ACTIVE,
                generation
        );
    }

    @Transactional
    public void clearChunksForRetry(Long documentId) {
        DocumentEntity document = findDocumentOrThrow(documentId);
        documentChunkRepository.deleteByDocumentId(documentId);
        document.setStatus(DocumentStatus.UPLOADED);
        document.setChunkCount(0);
        document.setErrorMessage(null);
        documentRepository.save(document);
    }

    @Transactional(readOnly = true)
    public List<DocumentSummaryResponse> listDocuments() {
        return documentRepository.findAllByLifecycleStatusOrderByCreatedAtDesc(DocumentLifecycleStatus.ACTIVE)
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
        DocumentEntity document = findDocumentOrThrow(documentId);
        int generation = document.getCurrentGeneration() == null ? 1 : document.getCurrentGeneration();
        return documentChunkRepository.findByDocumentIdAndChunkStatusAndGenerationOrderByChunkIndexAsc(
                        documentId,
                        ChunkStatus.ACTIVE,
                        generation
                )
                .stream()
                .map(this::toChunkResponse)
                .toList();
    }

    @Transactional
    public DocumentDeleteResponse softDeleteDocument(Long documentId) {
        return documentTrashService.moveToTrash(documentId);
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
        DocumentLifecycleStatus lifecycleStatus = document.getLifecycleStatus() == null
                ? DocumentLifecycleStatus.ACTIVE
                : document.getLifecycleStatus();
        boolean active = lifecycleStatus == DocumentLifecycleStatus.ACTIVE;
        boolean canReindex = active && hasUsableSourceText(document);
        String reindexDisabledReason = null;
        if (!active) {
            reindexDisabledReason = "当前文档已在垃圾箱中，不能重新索引";
        } else if (!hasUsableSourceText(document)) {
            reindexDisabledReason = "当前文档缺少原始文本，无法重新索引";
        }
        int currentGeneration = document.getCurrentGeneration() == null ? 1 : document.getCurrentGeneration();
        boolean canAsk = active && document.getStatus() == DocumentStatus.READY;
        List<CollectionMembershipResponse> memberships = collectionService.findMembershipsByDocumentId(document.getId());
        List<Long> collectionIds = memberships.stream().map(CollectionMembershipResponse::getCollectionId).toList();
        List<String> collectionNames = memberships.stream().map(CollectionMembershipResponse::getName).toList();
        return new DocumentSummaryResponse(
                document.getId(),
                document.getOriginalFilename(),
                document.getChunkCount(),
                lifecycleStatus.name(),
                DocumentLifecycleDisplayTexts.displayStatus(lifecycleStatus),
                document.getStatus().name(),
                DocumentLifecycleDisplayTexts.displayProcessingStatus(document.getStatus()),
                DocumentLifecycleDisplayTexts.lifecycleHint(lifecycleStatus, document.getStatus(), canAsk),
                document.getDeletedAt(),
                active,
                canAsk,
                currentGeneration,
                document.getReindexCount() == null ? 0 : document.getReindexCount(),
                document.getLastReindexedAt(),
                canReindex,
                reindexDisabledReason,
                memberships,
                collectionIds,
                collectionNames,
                true,
                !memberships.isEmpty(),
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
