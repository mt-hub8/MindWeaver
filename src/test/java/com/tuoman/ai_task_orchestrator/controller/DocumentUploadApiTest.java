package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.common.error.GlobalExceptionHandler;
import com.tuoman.ai_task_orchestrator.dto.DocumentIngestionResponse;
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
    void uploadShouldReturnIngestionSummary() throws Exception {
        when(documentIngestionService.ingest(any())).thenReturn(new DocumentIngestionResponse(
                42L,
                "demo.txt",
                "READY",
                3,
                3,
                3
        ));

        mockMvc.perform(multipart("/documents/upload")
                        .file(new MockMultipartFile(
                                "file",
                                "demo.txt",
                                "text/plain",
                                "cache key content".getBytes()
                        )))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.documentId").value(42))
                .andExpect(jsonPath("$.title").value("demo.txt"))
                .andExpect(jsonPath("$.status").value("READY"))
                .andExpect(jsonPath("$.chunkCount").value(3))
                .andExpect(jsonPath("$.embeddingCount").value(3))
                .andExpect(jsonPath("$.vectorWriteCount").value(3));
    }

    @Test
    void uploadShouldReturnValidationError() throws Exception {
        when(documentIngestionService.ingest(any()))
                .thenThrow(BusinessException.validationError("File must not be empty"));

        mockMvc.perform(multipart("/documents/upload")
                        .file(new MockMultipartFile("file", "demo.txt", "text/plain", new byte[0]))
                        .contentType(MediaType.MULTIPART_FORM_DATA))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("VALIDATION_ERROR"))
                .andExpect(jsonPath("$.message").value("File must not be empty"));
    }
}
