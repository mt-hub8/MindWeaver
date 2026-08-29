package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.common.error.GlobalExceptionHandler;
import com.tuoman.ai_task_orchestrator.config.BatchIngestionProperties;
import com.tuoman.ai_task_orchestrator.dto.BatchUploadResponse;
import com.tuoman.ai_task_orchestrator.dto.UploadBatchDetailResponse;
import com.tuoman.ai_task_orchestrator.dto.UploadBatchItemResponse;
import com.tuoman.ai_task_orchestrator.dto.UploadBatchSummaryResponse;
import com.tuoman.ai_task_orchestrator.service.UploadBatchService;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.context.annotation.Import;
import org.springframework.http.MediaType;
import org.springframework.mock.web.MockMultipartFile;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.test.web.servlet.MockMvc;

import java.time.LocalDateTime;
import java.util.List;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.multipart;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@WebMvcTest(controllers = BatchUploadController.class)
@Import(GlobalExceptionHandler.class)
class BatchUploadControllerTest {

    @Autowired
    private MockMvc mockMvc;

    @MockitoBean
    private UploadBatchService uploadBatchService;

    @MockitoBean
    private BatchIngestionProperties batchIngestionProperties;

    @Test
    void uploadShouldCreateBatch() throws Exception {
        when(uploadBatchService.createBatchUpload(any(), any(), any(), any())).thenReturn(
                new BatchUploadResponse(1L, "QUEUED", "排队中", 2, 2, 0, 0, "已创建批量导入任务")
        );

        mockMvc.perform(multipart("/documents/batches/upload")
                        .file(new MockMultipartFile("files", "a.txt", "text/plain", "a".getBytes()))
                        .file(new MockMultipartFile("files", "b.txt", "text/plain", "b".getBytes()))
                        .contentType(MediaType.MULTIPART_FORM_DATA))
                .andExpect(status().isAccepted())
                .andExpect(jsonPath("$.batchId").value(1))
                .andExpect(jsonPath("$.message").value("已创建批量导入任务"));
    }

    @Test
    void listBatchesShouldReturnSummaries() throws Exception {
        when(uploadBatchService.listBatches()).thenReturn(List.of(summary(1L)));

        mockMvc.perform(get("/documents/batches"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$[0].batchId").value(1));
    }

    @Test
    void getBatchDetailShouldReturnItems() throws Exception {
        when(uploadBatchService.getBatchDetail(1L)).thenReturn(new UploadBatchDetailResponse(
                summary(1L),
                List.of(new UploadBatchItemResponse(
                        10L, 1L, 100L, 200L, "demo.txt", "COMPLETED", "已完成",
                        12L, null, null, null, null, 0, "hash", "thash",
                        LocalDateTime.now(), LocalDateTime.now()
                ))
        ));

        mockMvc.perform(get("/documents/batches/1"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.batch.batchId").value(1))
                .andExpect(jsonPath("$.items[0].originalFilename").value("demo.txt"));
    }

    @Test
    void listItemsShouldReturnBatchItems() throws Exception {
        when(uploadBatchService.listItems(1L)).thenReturn(List.of());

        mockMvc.perform(get("/documents/batches/1/items"))
                .andExpect(status().isOk());
    }

    @Test
    void retryFailedShouldInvokeService() throws Exception {
        when(uploadBatchService.retryFailed(1L)).thenReturn(summary(1L));

        mockMvc.perform(post("/documents/batches/1/retry-failed"))
                .andExpect(status().isOk());

        verify(uploadBatchService).retryFailed(1L);
    }

    @Test
    void cancelShouldInvokeService() throws Exception {
        when(uploadBatchService.cancelBatch(1L)).thenReturn(summary(1L));

        mockMvc.perform(post("/documents/batches/1/cancel"))
                .andExpect(status().isOk());

        verify(uploadBatchService).cancelBatch(1L);
    }

    private UploadBatchSummaryResponse summary(Long id) {
        return new UploadBatchSummaryResponse(
                id, "批次", "QUEUED", "排队中",
                1, 0, 1, 0, 0, 0, 0, 0, 0,
                0, null, LocalDateTime.now(), null
        );
    }
}
