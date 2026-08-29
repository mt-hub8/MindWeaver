package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.document.ingestion.DocumentIngestionEventRecorder;
import com.tuoman.ai_task_orchestrator.dto.TrashCleanupResponse;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;

import java.time.Clock;
import java.time.LocalDateTime;
import java.util.List;

@Slf4j
@Service
@RequiredArgsConstructor
public class TrashCleanupService {

    private final DocumentRepository documentRepository;

    private final DocumentTrashService documentTrashService;

    private final DocumentIngestionEventRecorder documentIngestionEventRecorder;

    private final Clock clock;

    public TrashCleanupResponse purgeExpired() {
        LocalDateTime now = LocalDateTime.now(clock);
        List<DocumentEntity> expired = documentRepository.findByLifecycleStatusAndPurgeAfterBefore(
                DocumentLifecycleStatus.TRASHED,
                now
        );

        documentIngestionEventRecorder.recordTrashCleanupStarted(expired.size());

        int successCount = 0;
        int failureCount = 0;
        for (DocumentEntity document : expired) {
            try {
                documentTrashService.purge(document.getId());
                successCount++;
            } catch (Exception exception) {
                failureCount++;
                log.warn("Failed to purge expired trash documentId={}", document.getId(), exception);
            }
        }

        String message = "自动清理完成：成功 " + successCount + " 个，失败 " + failureCount + " 个";
        documentIngestionEventRecorder.recordTrashCleanupCompleted(successCount, failureCount);
        return new TrashCleanupResponse(successCount, failureCount, message);
    }
}
