package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.document.extract.TestDocumentFiles;
import com.tuoman.ai_task_orchestrator.dto.BatchUploadResponse;
import com.tuoman.ai_task_orchestrator.entity.UploadBatchItemEntity;
import com.tuoman.ai_task_orchestrator.enums.UploadBatchItemStatus;
import com.tuoman.ai_task_orchestrator.mq.DocumentIngestionMessage;
import com.tuoman.ai_task_orchestrator.mq.DocumentIngestionMessagePublisher;
import com.tuoman.ai_task_orchestrator.repository.UploadBatchItemRepository;
import com.tuoman.ai_task_orchestrator.repository.UploadBatchRepository;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.transaction.annotation.Transactional;
import org.springframework.web.multipart.MultipartFile;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.doAnswer;

@SpringBootTest(properties = {
        "app.batch-ingestion.staging-dir=target/test-staging/batches"
})
@Transactional
class BatchIngestionIntegrationTest {

    @Autowired
    private UploadBatchService uploadBatchService;

    @Autowired
    private UploadBatchRepository uploadBatchRepository;

    @Autowired
    private UploadBatchItemRepository uploadBatchItemRepository;

    @MockitoBean
    private DocumentIngestionMessagePublisher documentIngestionMessagePublisher;

    @Test
    void batchUploadShouldCreateItemsAndQueueIngestionTasks() {
        doAnswer(invocation -> {
            DocumentIngestionMessage message = invocation.getArgument(0);
            assertThat(message.getTaskId()).isNotNull();
            return null;
        }).when(documentIngestionMessagePublisher).publish(any());

        MultipartFile[] files = new MultipartFile[]{
                TestDocumentFiles.txtFile("batch-a.txt", "batch content alpha"),
                TestDocumentFiles.txtFile("batch-b.txt", "batch content beta")
        };

        BatchUploadResponse response = uploadBatchService.createBatchUpload(files, null, "集成测试批次", null);
        assertThat(response.getBatchId()).isNotNull();
        assertThat(response.getTotalCount()).isEqualTo(2);

        List<UploadBatchItemEntity> items = uploadBatchItemRepository.findByBatchIdOrderByIdAsc(response.getBatchId());
        assertThat(items).hasSize(2);
        assertThat(items.stream().filter(i -> i.getStatus() == UploadBatchItemStatus.QUEUED
                || i.getStatus() == UploadBatchItemStatus.PROCESSING
                || i.getStatus() == UploadBatchItemStatus.PENDING).count()).isGreaterThanOrEqualTo(1);
        assertThat(uploadBatchRepository.findById(response.getBatchId())).isPresent();
    }
}
