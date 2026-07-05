package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.config.TrashProperties;
import com.tuoman.ai_task_orchestrator.document.ingestion.DocumentIngestionEventRecorder;
import com.tuoman.ai_task_orchestrator.document.lifecycle.DocumentLifecycleDisplayTexts;
import com.tuoman.ai_task_orchestrator.dto.CollectionMembershipResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentDeleteResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentPurgeResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentRestoreResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentTrashItemResponse;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentPurgeStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.storage.StorageCleanupService;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.Clock;
import java.time.LocalDateTime;
import java.time.temporal.ChronoUnit;
import java.util.List;

@Slf4j
@Service
@RequiredArgsConstructor
public class DocumentTrashService {

    private final DocumentRepository documentRepository;

    private final CollectionService collectionService;

    private final TrashProperties trashProperties;

    private final StorageCleanupService storageCleanupService;

    private final DocumentIngestionEventRecorder documentIngestionEventRecorder;

    private final Clock clock;

    @Transactional
    public DocumentDeleteResponse moveToTrash(Long documentId) {
        DocumentEntity document = findDocumentOrThrow(documentId);

        if (document.getLifecycleStatus() == DocumentLifecycleStatus.PURGED) {
            throw BusinessException.documentAlreadyPurged();
        }

        if (document.getLifecycleStatus() == DocumentLifecycleStatus.TRASHED) {
            return new DocumentDeleteResponse(
                    document.getId(),
                    DocumentLifecycleStatus.TRASHED.name(),
                    DocumentLifecycleDisplayTexts.displayStatus(DocumentLifecycleStatus.TRASHED),
                    DocumentLifecycleDisplayTexts.deleteAlreadyTrashedMessage(),
                    document.getTrashedAt() != null ? document.getTrashedAt() : document.getDeletedAt()
            );
        }

        LocalDateTime now = LocalDateTime.now(clock);
        LocalDateTime purgeAfter = now.plusDays(trashProperties.getRetentionDays());

        document.setLifecycleStatus(DocumentLifecycleStatus.TRASHED);
        document.setDeletedAt(now);
        document.setTrashedAt(now);
        document.setPurgeAfter(purgeAfter);
        document.setPurgeStatus(DocumentPurgeStatus.NONE);
        documentRepository.save(document);

        documentIngestionEventRecorder.recordDocumentTrashed(documentId, purgeAfter);
        log.info("Document moved to trash, documentId={}, purgeAfter={}", documentId, purgeAfter);

        return new DocumentDeleteResponse(
                document.getId(),
                DocumentLifecycleStatus.TRASHED.name(),
                DocumentLifecycleDisplayTexts.displayStatus(DocumentLifecycleStatus.TRASHED),
                DocumentLifecycleDisplayTexts.deleteSuccessMessage(),
                now
        );
    }

    @Transactional
    public DocumentRestoreResponse restore(Long documentId) {
        DocumentEntity document = findDocumentOrThrow(documentId);

        if (document.getLifecycleStatus() == DocumentLifecycleStatus.PURGED) {
            throw BusinessException.documentRestoreNotAllowed("已永久删除的文档不可恢复");
        }
        if (document.getLifecycleStatus() != DocumentLifecycleStatus.TRASHED) {
            throw BusinessException.documentNotTrashed();
        }

        LocalDateTime now = LocalDateTime.now(clock);
        document.setLifecycleStatus(DocumentLifecycleStatus.ACTIVE);
        document.setPurgeAfter(null);
        document.setPurgeStatus(DocumentPurgeStatus.NONE);
        documentRepository.save(document);

        documentIngestionEventRecorder.recordDocumentRestored(documentId);
        log.info("Document restored from trash, documentId={}", documentId);

        return new DocumentRestoreResponse(
                documentId,
                DocumentLifecycleStatus.ACTIVE.name(),
                DocumentLifecycleDisplayTexts.displayStatus(DocumentLifecycleStatus.ACTIVE),
                DocumentLifecycleDisplayTexts.restoreSuccessMessage(),
                now
        );
    }

    @Transactional
    public DocumentPurgeResponse purge(Long documentId) {
        DocumentEntity document = findDocumentOrThrow(documentId);

        if (document.getLifecycleStatus() == DocumentLifecycleStatus.PURGED) {
            return new DocumentPurgeResponse(
                    documentId,
                    DocumentLifecycleStatus.PURGED.name(),
                    DocumentLifecycleDisplayTexts.displayStatus(DocumentLifecycleStatus.PURGED),
                    "该文档已永久删除",
                    document.getPurgedAt(),
                    List.of()
            );
        }
        if (document.getLifecycleStatus() == DocumentLifecycleStatus.ACTIVE) {
            throw BusinessException.documentPurgeNotAllowed("请先放入垃圾箱，再执行永久删除");
        }
        if (document.getLifecycleStatus() != DocumentLifecycleStatus.TRASHED) {
            throw BusinessException.documentPurgeNotAllowed("当前状态不允许永久删除");
        }

        document.setPurgeStatus(DocumentPurgeStatus.PURGING);
        documentRepository.save(document);
        documentIngestionEventRecorder.recordDocumentPurging(documentId);

        List<String> warnings;
        try {
            warnings = storageCleanupService.purgeDocumentStorage(document);
        } catch (Exception exception) {
            document.setPurgeStatus(DocumentPurgeStatus.FAILED);
            documentRepository.save(document);
            documentIngestionEventRecorder.recordDocumentPurgeFailed(documentId, exception.getMessage());
            throw BusinessException.documentPurgeFailed(exception.getMessage());
        }

        LocalDateTime now = LocalDateTime.now(clock);
        document.setLifecycleStatus(DocumentLifecycleStatus.PURGED);
        document.setPurgedAt(now);
        document.setPurgeStatus(DocumentPurgeStatus.PURGED);
        document.setPurgeAfter(null);
        documentRepository.save(document);

        documentIngestionEventRecorder.recordDocumentPurged(documentId, warnings);
        log.info("Document purged, documentId={}, warnings={}", documentId, warnings.size());

        return new DocumentPurgeResponse(
                documentId,
                DocumentLifecycleStatus.PURGED.name(),
                DocumentLifecycleDisplayTexts.displayStatus(DocumentLifecycleStatus.PURGED),
                DocumentLifecycleDisplayTexts.purgeSuccessMessage(),
                now,
                warnings
        );
    }

    @Transactional(readOnly = true)
    public List<DocumentTrashItemResponse> listTrash() {
        return documentRepository.findByLifecycleStatusOrderByTrashedAtDesc(DocumentLifecycleStatus.TRASHED)
                .stream()
                .map(this::toTrashItem)
                .toList();
    }

    public int remainingRetentionDays(DocumentEntity document) {
        if (document.getPurgeAfter() == null) {
            return trashProperties.getRetentionDays();
        }
        long days = ChronoUnit.DAYS.between(LocalDateTime.now(clock).toLocalDate(), document.getPurgeAfter().toLocalDate());
        return (int) Math.max(0, days);
    }

    private DocumentTrashItemResponse toTrashItem(DocumentEntity document) {
        List<CollectionMembershipResponse> memberships = collectionService.findMembershipsByDocumentId(document.getId());
        List<String> collectionNames = memberships.stream().map(CollectionMembershipResponse::getName).toList();
        Long sizeBytes = document.getFileSize();
        return new DocumentTrashItemResponse(
                document.getId(),
                document.getOriginalFilename(),
                DocumentLifecycleStatus.TRASHED.name(),
                DocumentLifecycleDisplayTexts.displayStatus(DocumentLifecycleStatus.TRASHED),
                document.getTrashedAt() != null ? document.getTrashedAt() : document.getDeletedAt(),
                document.getPurgeAfter(),
                remainingRetentionDays(document),
                sizeBytes,
                DocumentLifecycleDisplayTexts.formatBytes(sizeBytes),
                collectionNames,
                true,
                true
        );
    }

    private DocumentEntity findDocumentOrThrow(Long documentId) {
        return documentRepository.findById(documentId)
                .orElseThrow(BusinessException::documentNotFound);
    }
}
