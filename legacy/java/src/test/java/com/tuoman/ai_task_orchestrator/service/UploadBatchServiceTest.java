package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.entity.UploadBatchEntity;
import com.tuoman.ai_task_orchestrator.entity.UploadBatchItemEntity;
import com.tuoman.ai_task_orchestrator.enums.UploadBatchItemStatus;
import com.tuoman.ai_task_orchestrator.enums.UploadBatchStatus;
import com.tuoman.ai_task_orchestrator.repository.UploadBatchItemRepository;
import com.tuoman.ai_task_orchestrator.repository.UploadBatchRepository;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.transaction.annotation.Transactional;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest
@Transactional
class UploadBatchServiceTest {

    @Autowired
    private UploadBatchService uploadBatchService;

    @Autowired
    private UploadBatchRepository uploadBatchRepository;

    @Autowired
    private UploadBatchItemRepository uploadBatchItemRepository;

    @Test
    void shouldCreateBatchAndAggregateCounts() {
        UploadBatchEntity batch = new UploadBatchEntity();
        batch.setName("测试批次");
        batch.setStatus(UploadBatchStatus.CREATED);
        batch.setTotalCount(3);
        batch.initCounts();
        UploadBatchEntity saved = uploadBatchRepository.save(batch);

        uploadBatchItemRepository.save(item(saved.getId(), UploadBatchItemStatus.COMPLETED));
        uploadBatchItemRepository.save(item(saved.getId(), UploadBatchItemStatus.FAILED));
        uploadBatchItemRepository.save(item(saved.getId(), UploadBatchItemStatus.SKIPPED_DUPLICATE_FILE));

        uploadBatchService.refreshBatchCounts(saved.getId());
        UploadBatchEntity refreshed = uploadBatchRepository.findById(saved.getId()).orElseThrow();

        assertThat(refreshed.getCompletedCount()).isEqualTo(1);
        assertThat(refreshed.getFailedCount()).isEqualTo(1);
        assertThat(refreshed.getSkippedCount()).isEqualTo(1);
        assertThat(refreshed.getDuplicateCount()).isEqualTo(1);
    }

    @Test
    void cancelShouldMarkPendingItemsCanceledAndLeaveProcessing() {
        UploadBatchEntity batch = new UploadBatchEntity();
        batch.setName("取消测试");
        batch.setStatus(UploadBatchStatus.PROCESSING);
        batch.setTotalCount(2);
        batch.initCounts();
        UploadBatchEntity saved = uploadBatchRepository.save(batch);

        UploadBatchItemEntity pending = item(saved.getId(), UploadBatchItemStatus.PENDING);
        UploadBatchItemEntity processing = item(saved.getId(), UploadBatchItemStatus.PROCESSING);
        uploadBatchItemRepository.save(pending);
        uploadBatchItemRepository.save(processing);

        uploadBatchService.cancelBatch(saved.getId());

        List<UploadBatchItemEntity> items = uploadBatchItemRepository.findByBatchIdOrderByIdAsc(saved.getId());
        assertThat(items).anyMatch(i -> i.getStatus() == UploadBatchItemStatus.CANCELED);
        assertThat(items).anyMatch(i -> i.getStatus() == UploadBatchItemStatus.PROCESSING);
    }

    @Test
    void allCompletedItemsShouldFinalizeToCompleted() {
        UploadBatchEntity batch = new UploadBatchEntity();
        batch.setName("完成测试");
        batch.setStatus(UploadBatchStatus.PROCESSING);
        batch.setTotalCount(2);
        batch.initCounts();
        UploadBatchEntity saved = uploadBatchRepository.save(batch);
        uploadBatchItemRepository.save(item(saved.getId(), UploadBatchItemStatus.COMPLETED));
        uploadBatchItemRepository.save(item(saved.getId(), UploadBatchItemStatus.COMPLETED));

        uploadBatchService.tryFinalizeBatch(saved.getId());

        UploadBatchEntity refreshed = uploadBatchRepository.findById(saved.getId()).orElseThrow();
        assertThat(refreshed.getStatus()).isEqualTo(UploadBatchStatus.COMPLETED);
    }

    @Test
    void mixedFailedAndCompletedShouldFinalizeToPartialFailed() {
        UploadBatchEntity batch = new UploadBatchEntity();
        batch.setName("部分失败");
        batch.setStatus(UploadBatchStatus.PROCESSING);
        batch.setTotalCount(2);
        batch.initCounts();
        UploadBatchEntity saved = uploadBatchRepository.save(batch);
        uploadBatchItemRepository.save(item(saved.getId(), UploadBatchItemStatus.COMPLETED));
        uploadBatchItemRepository.save(item(saved.getId(), UploadBatchItemStatus.FAILED));

        uploadBatchService.tryFinalizeBatch(saved.getId());

        assertThat(uploadBatchRepository.findById(saved.getId()).orElseThrow().getStatus())
                .isEqualTo(UploadBatchStatus.PARTIAL_FAILED);
    }

    private UploadBatchItemEntity item(Long batchId, UploadBatchItemStatus status) {
        UploadBatchItemEntity entity = new UploadBatchItemEntity();
        entity.setBatchId(batchId);
        entity.setOriginalFilename("demo.txt");
        entity.setStatus(status);
        entity.setRetryCount(0);
        return entity;
    }
}
