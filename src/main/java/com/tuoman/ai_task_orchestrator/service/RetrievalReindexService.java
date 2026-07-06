package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.config.ChunkingProperties;
import com.tuoman.ai_task_orchestrator.dto.DocumentReindexSubmitResponse;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.entity.RetrievalReindexEventEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentCollectionRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.repository.RetrievalReindexEventRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.ArrayList;
import java.util.List;

@Service
@RequiredArgsConstructor
public class RetrievalReindexService {

    private final DocumentReindexService documentReindexService;

    private final DocumentRepository documentRepository;

    private final DocumentCollectionRepository documentCollectionRepository;

    private final RetrievalReindexEventRepository reindexEventRepository;

    private final ChunkingProperties chunkingProperties;

    @Transactional
    public DocumentReindexSubmitResponse reindexDocument(Long documentId, boolean forceRechunk) {
        DocumentEntity document = documentRepository.findById(documentId)
                .orElseThrow(BusinessException::documentNotFound);
        ensureReindexable(document);
        recordEvent("DOCUMENT", documentId, documentId, null, "SUBMITTED", "document reindex requested");
        return documentReindexService.submitReindex(documentId);
    }

    @Transactional
    public List<DocumentReindexSubmitResponse> reindexCollection(Long collectionId) {
        List<Long> documentIds = documentCollectionRepository.findAskableDocumentIdsByCollectionId(collectionId);
        List<DocumentReindexSubmitResponse> responses = new ArrayList<>();
        for (Long documentId : documentIds) {
            DocumentEntity document = documentRepository.findById(documentId).orElse(null);
            if (document == null || document.getLifecycleStatus() != DocumentLifecycleStatus.ACTIVE) {
                continue;
            }
            responses.add(reindexDocument(documentId, true));
        }
        recordEvent("COLLECTION", collectionId, null, collectionId, "SUBMITTED",
                "collection reindex requested for " + responses.size() + " documents");
        return responses;
    }

    @Transactional
    public List<DocumentReindexSubmitResponse> reindexAllActive() {
        List<DocumentReindexSubmitResponse> responses = new ArrayList<>();
        for (DocumentEntity document : documentRepository.findAll()) {
            if (document.getLifecycleStatus() != DocumentLifecycleStatus.ACTIVE) {
                continue;
            }
            try {
                responses.add(reindexDocument(document.getId(), true));
            } catch (BusinessException exception) {
                recordEvent("GLOBAL", null, document.getId(), null, "SKIPPED", exception.getMessage());
            }
        }
        recordEvent("GLOBAL", null, null, null, "SUBMITTED", "global reindex requested");
        return responses;
    }

    private void ensureReindexable(DocumentEntity document) {
        if (document.getLifecycleStatus() == DocumentLifecycleStatus.TRASHED
                || document.getLifecycleStatus() == DocumentLifecycleStatus.PURGED) {
            throw BusinessException.documentDeletedCannotReindex();
        }
    }

    private void recordEvent(String scopeType, Long scopeId, Long documentId, Long collectionId, String status, String message) {
        RetrievalReindexEventEntity event = new RetrievalReindexEventEntity();
        event.setScopeType(scopeType);
        event.setScopeId(scopeId);
        event.setDocumentId(documentId);
        event.setCollectionId(collectionId);
        event.setChunkingStrategy(chunkingProperties.getStrategy() == null ? null : chunkingProperties.getStrategy().name());
        event.setStatus(status);
        event.setMessage(message);
        reindexEventRepository.save(event);
    }
}
