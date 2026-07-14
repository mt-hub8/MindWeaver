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
import com.tuoman.ai_task_orchestrator.vectorindex.VectorLifecycleSyncService;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.Clock;
import java.time.LocalDateTime;
import java.time.temporal.ChronoUnit;
import java.util.List;

/**
 * V12 文档 Trash / Restore / Purge 生命周期服务。
 *
 * ACTIVE 表示可检索，TRASHED 表示进入垃圾箱且默认不参与检索，PURGED 表示永久删除。
 * Restore 只恢复生命周期状态；Purge 才执行原文、chunk、embedding、vector、membership 的物理清理。
 */
@Slf4j
@Service
@RequiredArgsConstructor
public class DocumentTrashService {

    private final DocumentRepository documentRepository;

    private final CollectionService collectionService;

    private final TrashProperties trashProperties;

    private final StorageCleanupService storageCleanupService;

    private final VectorLifecycleSyncService vectorLifecycleSyncService;

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

        // TRASHED 不立即物理删除 vector。
        // 这样用户 restore 时不需要重新 embedding，但检索链路必须通过 status filter 排除。
        document.setLifecycleStatus(DocumentLifecycleStatus.TRASHED);
        document.setDeletedAt(now);
        document.setTrashedAt(now);
        document.setPurgeAfter(purgeAfter);
        document.setPurgeStatus(DocumentPurgeStatus.NONE);
        documentRepository.save(document);

        documentIngestionEventRecorder.recordDocumentTrashed(documentId, purgeAfter);
        // 同步向量 payload/status，确保 finalTopK、citation、prompt context 都不会包含 TRASHED 文档。
        vectorLifecycleSyncService.syncTrash(documentId);
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
        // restore 只把 lifecycle 恢复为 ACTIVE。
        // 因为 TRASHED 期间向量未物理删除，所以通常不需要重新 embedding。
        document.setLifecycleStatus(DocumentLifecycleStatus.ACTIVE);
        document.setPurgeAfter(null);
        document.setPurgeStatus(DocumentPurgeStatus.NONE);
        documentRepository.save(document);

        documentIngestionEventRecorder.recordDocumentRestored(documentId);
        vectorLifecycleSyncService.syncRestore(documentId);
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

        // PURGED 是不可恢复删除，必须先进入 PURGING 并记录失败状态。
        // 如果清理 vector/storage 失败，不能把文档标成已永久删除。
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
