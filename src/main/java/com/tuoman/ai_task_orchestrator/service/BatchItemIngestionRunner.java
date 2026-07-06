package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.config.BatchIngestionProperties;
import com.tuoman.ai_task_orchestrator.entity.UploadBatchEntity;
import com.tuoman.ai_task_orchestrator.entity.UploadBatchItemEntity;
import com.tuoman.ai_task_orchestrator.enums.UploadBatchItemStatus;
import com.tuoman.ai_task_orchestrator.enums.UploadBatchStatus;
import com.tuoman.ai_task_orchestrator.repository.UploadBatchItemRepository;
import com.tuoman.ai_task_orchestrator.repository.UploadBatchRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.context.annotation.Lazy;
import org.springframework.stereotype.Service;

import java.util.List;

@Service
@RequiredArgsConstructor
public class BatchItemIngestionRunner {

    private final UploadBatchRepository uploadBatchRepository;

    private final UploadBatchItemRepository uploadBatchItemRepository;

    private final BatchIngestionProperties batchIngestionProperties;

    @Lazy
    private final UploadBatchService uploadBatchService;

    public void processBatch(Long batchId) {
        UploadBatchEntity batch = uploadBatchRepository.findById(batchId)
                .orElseThrow(() -> BusinessException.invalidRequest("批量导入批次不存在"));

        if (batch.getStatus() == UploadBatchStatus.CANCELED
                || batch.getStatus() == UploadBatchStatus.CANCEL_REQUESTED) {
            return;
        }

        List<UploadBatchItemEntity> pendingItems = uploadBatchItemRepository.findByBatchIdAndStatus(
                batchId,
                UploadBatchItemStatus.PENDING
        );
        int concurrency = Math.max(1, batchIngestionProperties.getDocumentParseConcurrency());
        long activeCount = uploadBatchItemRepository.countByBatchIdAndStatus(batchId, UploadBatchItemStatus.QUEUED)
                + uploadBatchItemRepository.countByBatchIdAndStatus(batchId, UploadBatchItemStatus.PROCESSING);
        int slots = (int) Math.max(0, concurrency - activeCount);
        int submitted = 0;
        for (UploadBatchItemEntity item : pendingItems) {
            UploadBatchEntity latestBatch = uploadBatchRepository.findById(batchId).orElse(batch);
            if (latestBatch.getStatus() == UploadBatchStatus.CANCEL_REQUESTED
                    || latestBatch.getStatus() == UploadBatchStatus.CANCELED) {
                break;
            }
            uploadBatchService.queueItemForIngestion(item, latestBatch);
            submitted++;
            if (submitted >= slots) {
                break;
            }
        }

        uploadBatchService.refreshBatchCounts(batchId);
        uploadBatchService.tryFinalizeBatch(batchId);
    }
}
