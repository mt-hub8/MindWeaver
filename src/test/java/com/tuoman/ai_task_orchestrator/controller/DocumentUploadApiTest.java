package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.common.error.GlobalExceptionHandler;
import com.tuoman.ai_task_orchestrator.dto.DocumentIngestionSubmitResponse;
import com.tuoman.ai_task_orchestrator.service.DocumentEmbeddingService;
import com.tuoman.ai_task_orchestrator.service.DocumentIngestionService;
import com.tuoman.ai_task_orchestrator.service.DocumentService;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.context.annotation.Import;
import org.springframework.http.MediaType;
import org.springframework.mock.web.MockMultipartFile;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.test.web.servlet.MockMvc;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.multipart;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@WebMvcTest(controllers = DocumentController.class)
@Import(GlobalExceptionHandler.class)
class DocumentUploadApiTest {

    @Autowired
    private MockMvc mockMvc;

    @MockitoBean
    private DocumentService documentService;

    @MockitoBean
    private DocumentEmbeddingService documentEmbeddingService;

    @MockitoBean
    private DocumentIngestionService documentIngestionService;

    @Test
    void uploadShouldReturnAcceptedIngestionTask() throws Exception {
        when(documentIngestionService.submitUpload(any())).thenReturn(new DocumentIngestionSubmitResponse(
                1001L,
                42L,
                "demo.txt",
                "PENDING",
                "待处理",
                "文档已提交，正在排队处理。"
        ));

        mockMvc.perform(multipart("/documents/upload")
                        .file(new MockMultipartFile(
                                "file",
                                "demo.txt",
                                "text/plain",
                                "cache key content".getBytes()
                        )))
                .andExpect(status().isAccepted())
                .andExpect(jsonPath("$.taskId").value(1001))
                .andExpect(jsonPath("$.documentId").value(42))
                .andExpect(jsonPath("$.filename").value("demo.txt"))
                .andExpect(jsonPath("$.status").value("PENDING"))
                .andExpect(jsonPath("$.displayStatus").value("待处理"))
                .andExpect(jsonPath("$.displayMessage").value("文档已提交，正在排队处理。"));
    }

    @Test
    void uploadShouldReturnValidationError() throws Exception {
        when(documentIngestionService.submitUpload(any()))
                .thenThrow(BusinessException.validationError("提取的文档文本不能为空"));

        mockMvc.perform(multipart("/documents/upload")
                        .file(new MockMultipartFile("file", "demo.txt", "text/plain", new byte[0]))
                        .contentType(MediaType.MULTIPART_FORM_DATA))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("VALIDATION_ERROR"))
                .andExpect(jsonPath("$.message").value("提取的文档文本不能为空"));
    }
}
