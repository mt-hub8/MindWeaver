package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.config.BatchIngestionProperties;
import com.tuoman.ai_task_orchestrator.dto.BatchIngestionConfigResponse;
import com.tuoman.ai_task_orchestrator.dto.BatchUploadResponse;
import com.tuoman.ai_task_orchestrator.dto.UploadBatchDetailResponse;
import com.tuoman.ai_task_orchestrator.dto.UploadBatchItemResponse;
import com.tuoman.ai_task_orchestrator.dto.UploadBatchSummaryResponse;
import com.tuoman.ai_task_orchestrator.enums.DuplicatePolicy;
import com.tuoman.ai_task_orchestrator.service.UploadBatchService;
import lombok.RequiredArgsConstructor;
import org.springframework.http.HttpStatus;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.ResponseStatus;
import org.springframework.web.bind.annotation.RestController;
import org.springframework.web.multipart.MultipartFile;

import java.util.List;

@RestController
@RequestMapping("/documents/batches")
@RequiredArgsConstructor
/**
 * 批量导入 HTTP 入口。
 *
 * 负责创建 UploadBatch、查看 batch/item 进度、重试失败项和取消批次；
 * 单个文件的实际摄入仍复用 DocumentIngestionService。
 */
public class BatchUploadController {

    private final UploadBatchService uploadBatchService;

    private final BatchIngestionProperties batchIngestionProperties;

    @PostMapping("/upload")
    @ResponseStatus(HttpStatus.ACCEPTED)
    public BatchUploadResponse uploadBatch(
            @RequestParam("files") MultipartFile[] files,
            @RequestParam(value = "collectionId", required = false) Long collectionId,
            @RequestParam(value = "batchName", required = false) String batchName,
            @RequestParam(value = "duplicatePolicy", required = false) DuplicatePolicy duplicatePolicy
    ) {
        return uploadBatchService.createBatchUpload(files, collectionId, batchName, duplicatePolicy);
    }

    @GetMapping
    public List<UploadBatchSummaryResponse> listBatches() {
        return uploadBatchService.listBatches();
    }

    @GetMapping("/config")
    public BatchIngestionConfigResponse getConfig() {
        return new BatchIngestionConfigResponse(
                batchIngestionProperties.isEnabled(),
                batchIngestionProperties.getMaxFilesPerBatch(),
                batchIngestionProperties.getDocumentParseConcurrency(),
                batchIngestionProperties.getEmbeddingConcurrency(),
                batchIngestionProperties.getMaxRetryCount(),
                batchIngestionProperties.getStagingDir()
        );
    }

    @GetMapping("/{batchId}")
    public UploadBatchDetailResponse getBatch(@PathVariable Long batchId) {
        return uploadBatchService.getBatchDetail(batchId);
    }

    @GetMapping("/{batchId}/items")
    public List<UploadBatchItemResponse> listItems(@PathVariable Long batchId) {
        return uploadBatchService.listItems(batchId);
    }

    @PostMapping("/{batchId}/retry-failed")
    public UploadBatchSummaryResponse retryFailed(@PathVariable Long batchId) {
        return uploadBatchService.retryFailed(batchId);
    }

    @PostMapping("/{batchId}/cancel")
    public UploadBatchSummaryResponse cancelBatch(@PathVariable Long batchId) {
        return uploadBatchService.cancelBatch(batchId);
    }
}
