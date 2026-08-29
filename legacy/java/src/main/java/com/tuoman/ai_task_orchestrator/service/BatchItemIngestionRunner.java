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

/**
 * 批量导入 item 调度器。
 *
 * 该类负责把 PENDING item 按配置并发数推进到 DocumentIngestionService，
 * 让 batch 可以在多个 ingestion task 完成后继续补位，而不是一次性打满 worker。
 */
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
        // activeCount 同时统计 QUEUED 和 PROCESSING，避免 MQ 已排队但尚未开始的任务被重复提交。
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
            // 每提交一个 item 前重新读取 batch 状态，确保 cancel 请求能尽快停止后续排队。
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
