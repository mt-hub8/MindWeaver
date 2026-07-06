package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.batch.BatchStagingService;
import com.tuoman.ai_task_orchestrator.batch.DuplicateDetectionService;
import com.tuoman.ai_task_orchestrator.batch.UploadBatchDisplayTexts;
import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.config.BatchIngestionProperties;
import com.tuoman.ai_task_orchestrator.document.ingestion.DocumentFileValidator;
import com.tuoman.ai_task_orchestrator.document.ingestion.FileHashService;
import com.tuoman.ai_task_orchestrator.document.extract.DocumentTextExtractorRegistry;
import com.tuoman.ai_task_orchestrator.dto.BatchUploadResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentIngestionSubmitResponse;
import com.tuoman.ai_task_orchestrator.dto.UploadBatchDetailResponse;
import com.tuoman.ai_task_orchestrator.dto.UploadBatchItemResponse;
import com.tuoman.ai_task_orchestrator.dto.UploadBatchSummaryResponse;
import com.tuoman.ai_task_orchestrator.entity.UploadBatchEntity;
import com.tuoman.ai_task_orchestrator.entity.UploadBatchItemEntity;
import com.tuoman.ai_task_orchestrator.enums.DuplicatePolicy;
import com.tuoman.ai_task_orchestrator.enums.UploadBatchItemStatus;
import com.tuoman.ai_task_orchestrator.enums.UploadBatchStatus;
import com.tuoman.ai_task_orchestrator.repository.UploadBatchItemRepository;
import com.tuoman.ai_task_orchestrator.repository.UploadBatchRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.beans.factory.ObjectProvider;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;
import org.springframework.web.multipart.MultipartFile;

import java.io.IOException;
import java.nio.file.Path;
import java.time.LocalDateTime;
import java.util.ArrayList;
import java.util.EnumSet;
import java.util.List;

@Service
@RequiredArgsConstructor
public class UploadBatchService {

    private static final EnumSet<UploadBatchItemStatus> TERMINAL_ITEM_STATUSES = EnumSet.of(
            UploadBatchItemStatus.COMPLETED,
            UploadBatchItemStatus.FAILED,
            UploadBatchItemStatus.SKIPPED_DUPLICATE_FILE,
            UploadBatchItemStatus.SKIPPED_DUPLICATE_TEXT,
            UploadBatchItemStatus.CANCELED
    );

    private final UploadBatchRepository uploadBatchRepository;

    private final UploadBatchItemRepository uploadBatchItemRepository;

    private final BatchIngestionProperties batchIngestionProperties;

    private final BatchStagingService batchStagingService;

    private final DuplicateDetectionService duplicateDetectionService;

    private final DocumentFileValidator documentFileValidator;

    private final DocumentTextExtractorRegistry documentTextExtractorRegistry;

    private final FileHashService fileHashService;

    private final DocumentIngestionService documentIngestionService;

    private final CollectionService collectionService;

    private final ObjectProvider<BatchItemIngestionRunner> batchItemIngestionRunnerProvider;

    private final NotificationService notificationService;

    @Transactional
    public BatchUploadResponse createBatchUpload(
            MultipartFile[] files,
            Long collectionId,
            String batchName,
            DuplicatePolicy duplicatePolicy
    ) {
        if (!batchIngestionProperties.isEnabled()) {
            throw BusinessException.validationError("批量导入功能未启用");
        }
        if (files == null || files.length == 0) {
            throw BusinessException.validationError("请至少选择一个文件");
        }
        if (files.length > batchIngestionProperties.getMaxFilesPerBatch()) {
            throw BusinessException.validationError("单批最多上传 " + batchIngestionProperties.getMaxFilesPerBatch() + " 个文件");
        }

        DuplicatePolicy policy = duplicatePolicy == null ? DuplicatePolicy.SKIP : duplicatePolicy;

        UploadBatchEntity batch = new UploadBatchEntity();
        batch.setName(batchName == null || batchName.isBlank() ? "批量导入 " + LocalDateTime.now() : batchName.trim());
        batch.setStatus(UploadBatchStatus.CREATED);
        batch.setDuplicatePolicy(policy);
        batch.setCollectionId(collectionId);
        batch.setTotalCount(files.length);
        batch.initCounts();
        batch.setPendingCount(files.length);
        UploadBatchEntity savedBatch = uploadBatchRepository.save(batch);

        List<UploadBatchItemEntity> createdItems = new ArrayList<>();
        for (MultipartFile file : files) {
            createdItems.add(createBatchItem(savedBatch, file, policy));
        }
        uploadBatchItemRepository.saveAll(createdItems);

        savedBatch.setStatus(UploadBatchStatus.QUEUED);
        if (savedBatch.getStartedAt() == null) {
            savedBatch.setStartedAt(LocalDateTime.now());
        }
        refreshBatchCounts(savedBatch.getId());

        batchItemIngestionRunnerProvider.getObject().processBatch(savedBatch.getId());

        UploadBatchEntity refreshed = uploadBatchRepository.findById(savedBatch.getId()).orElseThrow();
        return new BatchUploadResponse(
                refreshed.getId(),
                refreshed.getStatus().name(),
                UploadBatchDisplayTexts.displayBatchStatus(refreshed.getStatus()),
                refreshed.getTotalCount(),
                refreshed.getQueuedCount(),
                refreshed.getDuplicateCount(),
                refreshed.getSkippedCount(),
                "已创建批量导入任务"
        );
    }

    private UploadBatchItemEntity createBatchItem(UploadBatchEntity batch, MultipartFile file, DuplicatePolicy policy) {
        var fileType = documentFileValidator.validate(file);
        byte[] bytes;
        try {
            bytes = file.getBytes();
        } catch (IOException exception) {
            throw BusinessException.internalError("读取上传文件失败: " + file.getOriginalFilename());
        }
        String fileHash = fileHashService.hashBytes(bytes);

        UploadBatchItemEntity item = new UploadBatchItemEntity();
        item.setBatchId(batch.getId());
        item.setOriginalFilename(file.getOriginalFilename());
        item.setContentType(file.getContentType());
        item.setFileSize(file.getSize());
        item.setFileHash(fileHash);
        item.setStatus(UploadBatchItemStatus.PENDING);
        item = uploadBatchItemRepository.save(item);

        Path stagingPath = batchStagingService.saveStagingFile(
                batch.getId(),
                item.getId(),
                file.getOriginalFilename(),
                bytes
        );
        item.setStagingFilePath(stagingPath.toString());

        DuplicateDetectionService.FileDuplicateDecision decision =
                duplicateDetectionService.evaluateFileDuplicate(fileHash, policy);
        if (decision.duplicate() && decision.skip() && !decision.importAnyway()) {
            item.setStatus(decision.useExisting()
                    ? UploadBatchItemStatus.SKIPPED_DUPLICATE_FILE
                    : UploadBatchItemStatus.SKIPPED_DUPLICATE_FILE);
            item.setDuplicateOfDocumentId(decision.duplicateDocumentId());
            item.setSkipReason(decision.skipReason());
            item.setCompletedAt(LocalDateTime.now());
            if (decision.useExisting() && batch.getCollectionId() != null && decision.duplicateDocumentId() != null) {
                collectionService.assignDocument(batch.getCollectionId(), decision.duplicateDocumentId());
                item.setDocumentId(decision.duplicateDocumentId());
            }
            return item;
        }
        if (decision.importAnyway()) {
            item.setSkipReason(decision.skipReason());
        }

        try {
            String text = documentTextExtractorRegistry.extract(file, fileType);
            item.setTextHash(fileHashService.hashText(text));
        } catch (RuntimeException exception) {
            item.setStatus(UploadBatchItemStatus.FAILED);
            item.setFailureCode("TEXT_EXTRACT_FAILED");
            item.setFailureMessage(exception.getMessage());
            item.setCompletedAt(LocalDateTime.now());
        }
        return item;
    }

    @Transactional
    public void queueItemForIngestion(UploadBatchItemEntity item, UploadBatchEntity batch) {
        if (item.getStatus() != UploadBatchItemStatus.PENDING) {
            return;
        }
        if (batch.getStatus() == UploadBatchStatus.CANCEL_REQUESTED
                || batch.getStatus() == UploadBatchStatus.CANCELED) {
            item.setStatus(UploadBatchItemStatus.CANCELED);
            item.setCompletedAt(LocalDateTime.now());
            uploadBatchItemRepository.save(item);
            return;
        }

        byte[] bytes = batchStagingService.readStagingFile(item.getStagingFilePath());
        String text = documentTextExtractorRegistry.extractFromBytes(
                bytes,
                item.getOriginalFilename(),
                item.getContentType()
        );
        if (text == null || text.isBlank()) {
            markItemFailed(item, "VALIDATION_ERROR", "提取的文档文本不能为空");
            return;
        }

        item.setStatus(UploadBatchItemStatus.QUEUED);
        item.setStartedAt(LocalDateTime.now());
        uploadBatchItemRepository.save(item);

        try {
            DocumentIngestionSubmitResponse response = documentIngestionService.submitIngestion(
                    item.getOriginalFilename(),
                    item.getContentType(),
                    item.getFileSize() == null ? bytes.length : item.getFileSize(),
                    bytes,
                    text,
                    item.getId()
            );
            item.setDocumentId(response.getDocumentId());
            item.setIngestionTaskId(response.getTaskId());
            item.setStatus(UploadBatchItemStatus.PROCESSING);
            uploadBatchItemRepository.save(item);
            if (batch.getCollectionId() != null) {
                collectionService.assignDocument(batch.getCollectionId(), response.getDocumentId());
            }
        } catch (RuntimeException exception) {
            markItemFailed(item, "INGEST_SUBMIT_FAILED", exception.getMessage());
        }
    }

    @Transactional
    public void onIngestionTaskStarted(Long taskId) {
        uploadBatchItemRepository.findByIngestionTaskId(taskId).ifPresent(item -> {
            item.setStatus(UploadBatchItemStatus.PROCESSING);
            if (item.getStartedAt() == null) {
                item.setStartedAt(LocalDateTime.now());
            }
            uploadBatchItemRepository.save(item);
            refreshBatchCounts(item.getBatchId());
        });
    }

    @Transactional
    public void onIngestionTaskCompleted(Long taskId) {
        uploadBatchItemRepository.findByIngestionTaskId(taskId).ifPresent(item -> {
            item.setStatus(UploadBatchItemStatus.COMPLETED);
            item.setCompletedAt(LocalDateTime.now());
            uploadBatchItemRepository.save(item);
            batchItemIngestionRunnerProvider.getObject().processBatch(item.getBatchId());
            finalizeBatchIfReady(item.getBatchId());
        });
    }

    @Transactional
    public void onIngestionTaskFailed(Long taskId, String errorCode, String errorMessage) {
        uploadBatchItemRepository.findByIngestionTaskId(taskId).ifPresent(item -> {
            markItemFailed(item, errorCode, errorMessage);
            batchItemIngestionRunnerProvider.getObject().processBatch(item.getBatchId());
            finalizeBatchIfReady(item.getBatchId());
        });
    }

    @Transactional
    public void markTextDuplicateSkipped(Long batchItemId, Long duplicateDocumentId) {
        UploadBatchItemEntity item = uploadBatchItemRepository.findById(batchItemId)
                .orElseThrow(() -> BusinessException.invalidRequest("批量导入条目不存在"));
        item.setStatus(UploadBatchItemStatus.SKIPPED_DUPLICATE_TEXT);
        item.setDuplicateOfDocumentId(duplicateDocumentId);
        item.setSkipReason("检测到相同文本内容，已跳过重复 embedding。");
        item.setCompletedAt(LocalDateTime.now());
        uploadBatchItemRepository.save(item);
        batchItemIngestionRunnerProvider.getObject().processBatch(item.getBatchId());
        finalizeBatchIfReady(item.getBatchId());
    }

    @Transactional
    public UploadBatchDetailResponse getBatchDetail(Long batchId) {
        UploadBatchEntity batch = findBatchOrThrow(batchId);
        List<UploadBatchItemResponse> items = uploadBatchItemRepository.findByBatchIdOrderByIdAsc(batchId)
                .stream()
                .map(this::toItemResponse)
                .toList();
        return new UploadBatchDetailResponse(toSummaryResponse(batch), items);
    }

    public List<UploadBatchSummaryResponse> listBatches() {
        return uploadBatchRepository.findAllByOrderByCreatedAtDesc().stream()
                .map(this::toSummaryResponse)
                .toList();
    }

    public List<UploadBatchItemResponse> listItems(Long batchId) {
        findBatchOrThrow(batchId);
        return uploadBatchItemRepository.findByBatchIdOrderByIdAsc(batchId).stream()
                .map(this::toItemResponse)
                .toList();
    }

    @Transactional
    public UploadBatchSummaryResponse retryFailed(Long batchId) {
        UploadBatchEntity batch = findBatchOrThrow(batchId);
        List<UploadBatchItemEntity> failedItems = uploadBatchItemRepository.findByBatchIdAndStatus(
                batchId,
                UploadBatchItemStatus.FAILED
        );
        for (UploadBatchItemEntity item : failedItems) {
            if (item.getRetryCount() >= batchIngestionProperties.getMaxRetryCount()) {
                continue;
            }
            item.setRetryCount(item.getRetryCount() + 1);
            item.setStatus(UploadBatchItemStatus.PENDING);
            item.setFailureCode(null);
            item.setFailureMessage(null);
            item.setCompletedAt(null);
            uploadBatchItemRepository.save(item);
        }
        batch.setStatus(UploadBatchStatus.QUEUED);
        uploadBatchRepository.save(batch);
        batchItemIngestionRunnerProvider.getObject().processBatch(batchId);
        refreshBatchCounts(batchId);
        return toSummaryResponse(findBatchOrThrow(batchId));
    }

    @Transactional
    public UploadBatchSummaryResponse cancelBatch(Long batchId) {
        UploadBatchEntity batch = findBatchOrThrow(batchId);
        batch.setStatus(UploadBatchStatus.CANCEL_REQUESTED);
        uploadBatchRepository.save(batch);

        List<UploadBatchItemEntity> items = uploadBatchItemRepository.findByBatchIdOrderByIdAsc(batchId);
        for (UploadBatchItemEntity item : items) {
            if (item.getStatus() == UploadBatchItemStatus.PENDING
                    || item.getStatus() == UploadBatchItemStatus.QUEUED) {
                item.setStatus(UploadBatchItemStatus.CANCELED);
                item.setCompletedAt(LocalDateTime.now());
                uploadBatchItemRepository.save(item);
            }
        }
        finalizeBatchIfReady(batchId);
        return toSummaryResponse(findBatchOrThrow(batchId));
    }

    @Transactional
    public void refreshBatchCounts(Long batchId) {
        UploadBatchEntity batch = findBatchOrThrow(batchId);
        List<UploadBatchItemEntity> items = uploadBatchItemRepository.findByBatchIdOrderByIdAsc(batchId);
        int pending = 0;
        int queued = 0;
        int processing = 0;
        int completed = 0;
        int failed = 0;
        int skipped = 0;
        int duplicate = 0;
        int canceled = 0;
        for (UploadBatchItemEntity item : items) {
            switch (item.getStatus()) {
                case PENDING -> pending++;
                case QUEUED -> queued++;
                case PROCESSING -> processing++;
                case COMPLETED -> completed++;
                case FAILED -> failed++;
                case SKIPPED_DUPLICATE_FILE -> {
                    skipped++;
                    duplicate++;
                }
                case SKIPPED_DUPLICATE_TEXT -> {
                    skipped++;
                    duplicate++;
                }
                case CANCEL_REQUESTED, CANCELED -> canceled++;
            }
        }
        batch.setTotalCount(items.size());
        batch.setPendingCount(pending);
        batch.setQueuedCount(queued);
        batch.setProcessingCount(processing);
        batch.setCompletedCount(completed);
        batch.setFailedCount(failed);
        batch.setSkippedCount(skipped);
        batch.setDuplicateCount(duplicate);
        batch.setCanceledCount(canceled);
        uploadBatchRepository.save(batch);
    }

    @Transactional
    public void tryFinalizeBatch(Long batchId) {
        finalizeBatchIfReady(batchId);
    }

    private void finalizeBatchIfReady(Long batchId) {
        refreshBatchCounts(batchId);
        UploadBatchEntity batch = findBatchOrThrow(batchId);
        List<UploadBatchItemEntity> items = uploadBatchItemRepository.findByBatchIdOrderByIdAsc(batchId);
        boolean allTerminal = items.stream().allMatch(item -> TERMINAL_ITEM_STATUSES.contains(item.getStatus())
                || item.getStatus() == UploadBatchItemStatus.CANCEL_REQUESTED);
        boolean anyProcessing = items.stream().anyMatch(item ->
                item.getStatus() == UploadBatchItemStatus.PENDING
                        || item.getStatus() == UploadBatchItemStatus.QUEUED
                        || item.getStatus() == UploadBatchItemStatus.PROCESSING
                        || item.getStatus() == UploadBatchItemStatus.CANCEL_REQUESTED);
        if (anyProcessing) {
            batch.setStatus(batch.getStatus() == UploadBatchStatus.CANCEL_REQUESTED
                    ? UploadBatchStatus.CANCEL_REQUESTED
                    : UploadBatchStatus.PROCESSING);
            uploadBatchRepository.save(batch);
            return;
        }
        if (!allTerminal) {
            return;
        }

        int completed = batch.getCompletedCount();
        int failed = batch.getFailedCount();
        int skipped = batch.getSkippedCount();
        int canceled = batch.getCanceledCount();
        int total = batch.getTotalCount();

        UploadBatchStatus terminalStatus;
        if (batch.getStatus() == UploadBatchStatus.CANCEL_REQUESTED
                || (canceled > 0 && completed + failed + skipped + canceled >= total
                && batch.getProcessingCount() == 0 && batch.getPendingCount() == 0 && batch.getQueuedCount() == 0)) {
            terminalStatus = UploadBatchStatus.CANCELED;
            batch.setSummaryMessage("批量导入已取消。已成功导入的文档不会被删除。");
        } else if (failed > 0 && completed + skipped > 0) {
            terminalStatus = UploadBatchStatus.PARTIAL_FAILED;
            batch.setSummaryMessage(String.format(
                    "本次导入 %d 个文件，成功 %d 个，失败 %d 个，跳过 %d 个。",
                    total, completed, failed, skipped
            ));
        } else if (failed > 0) {
            terminalStatus = UploadBatchStatus.FAILED;
            batch.setSummaryMessage(String.format("本次导入 %d 个文件全部失败。", total));
        } else {
            terminalStatus = UploadBatchStatus.COMPLETED;
            batch.setSummaryMessage(String.format(
                    "本次导入 %d 个文件，成功 %d 个，跳过重复 %d 个。",
                    total, completed, skipped
            ));
        }
        batch.setStatus(terminalStatus);
        batch.setCompletedAt(LocalDateTime.now());
        uploadBatchRepository.save(batch);
        notificationService.notifyBatchFinished(batch);
    }

    private void markItemFailed(UploadBatchItemEntity item, String code, String message) {
        item.setStatus(UploadBatchItemStatus.FAILED);
        item.setFailureCode(code);
        item.setFailureMessage(message);
        item.setCompletedAt(LocalDateTime.now());
        uploadBatchItemRepository.save(item);
        refreshBatchCounts(item.getBatchId());
    }

    private UploadBatchEntity findBatchOrThrow(Long batchId) {
        return uploadBatchRepository.findById(batchId)
                .orElseThrow(() -> BusinessException.invalidRequest("批量导入批次不存在"));
    }

    private UploadBatchSummaryResponse toSummaryResponse(UploadBatchEntity batch) {
        int progress = 0;
        if (batch.getTotalCount() != null && batch.getTotalCount() > 0) {
            int done = batch.getCompletedCount() + batch.getFailedCount() + batch.getSkippedCount() + batch.getCanceledCount();
            progress = Math.min(100, (int) Math.round(done * 100.0 / batch.getTotalCount()));
        }
        return new UploadBatchSummaryResponse(
                batch.getId(),
                batch.getName(),
                batch.getStatus().name(),
                UploadBatchDisplayTexts.displayBatchStatus(batch.getStatus()),
                safe(batch.getTotalCount()),
                safe(batch.getPendingCount()),
                safe(batch.getQueuedCount()),
                safe(batch.getProcessingCount()),
                safe(batch.getCompletedCount()),
                safe(batch.getFailedCount()),
                safe(batch.getSkippedCount()),
                safe(batch.getDuplicateCount()),
                safe(batch.getCanceledCount()),
                progress,
                batch.getSummaryMessage(),
                batch.getCreatedAt(),
                batch.getCompletedAt()
        );
    }

    private UploadBatchItemResponse toItemResponse(UploadBatchItemEntity item) {
        return new UploadBatchItemResponse(
                item.getId(),
                item.getBatchId(),
                item.getDocumentId(),
                item.getIngestionTaskId(),
                item.getOriginalFilename(),
                item.getStatus().name(),
                UploadBatchDisplayTexts.displayItemStatus(item.getStatus()),
                item.getFileSize(),
                item.getFailureCode(),
                item.getFailureMessage(),
                item.getDuplicateOfDocumentId(),
                item.getSkipReason(),
                item.getRetryCount() == null ? 0 : item.getRetryCount(),
                item.getFileHash(),
                item.getTextHash(),
                item.getCreatedAt(),
                item.getCompletedAt()
        );
    }

    private int safe(Integer value) {
        return value == null ? 0 : value;
    }
}
