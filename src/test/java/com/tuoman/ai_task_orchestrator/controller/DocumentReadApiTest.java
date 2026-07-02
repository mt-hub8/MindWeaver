package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.common.error.GlobalExceptionHandler;
import com.tuoman.ai_task_orchestrator.dto.DocumentChunkResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentSummaryResponse;
import com.tuoman.ai_task_orchestrator.service.DocumentEmbeddingService;
import com.tuoman.ai_task_orchestrator.service.DocumentIngestionService;
import com.tuoman.ai_task_orchestrator.service.DocumentService;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.context.annotation.Import;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.test.web.servlet.MockMvc;

import java.time.LocalDateTime;
import java.util.List;

import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@WebMvcTest(controllers = DocumentController.class)
@Import(GlobalExceptionHandler.class)
class DocumentReadApiTest {

    @Autowired
    private MockMvc mockMvc;

    @MockitoBean
    private DocumentService documentService;

    @MockitoBean
    private DocumentEmbeddingService documentEmbeddingService;

    @MockitoBean
    private DocumentIngestionService documentIngestionService;

    @Test
    void listDocumentsShouldReturnEmptyArrayWhenNoData() throws Exception {
        when(documentService.listDocuments()).thenReturn(List.of());

        mockMvc.perform(get("/documents"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$").isArray())
                .andExpect(jsonPath("$.length()").value(0));
    }

    @Test
    void listDocumentsShouldReturnSummaryFields() throws Exception {
        LocalDateTime createdAt = LocalDateTime.of(2026, 7, 2, 10, 0);
        when(documentService.listDocuments()).thenReturn(List.of(
                new DocumentSummaryResponse(1L, "demo.md", 3, "CHUNKED", createdAt, createdAt)
        ));

        mockMvc.perform(get("/documents"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$[0].documentId").value(1))
                .andExpect(jsonPath("$[0].title").value("demo.md"))
                .andExpect(jsonPath("$[0].chunkCount").value(3))
                .andExpect(jsonPath("$[0].status").value("CHUNKED"));
    }

    @Test
    void getDocumentChunksShouldReturnChunkFields() throws Exception {
        when(documentService.getDocumentChunks(1L)).thenReturn(List.of(
                new DocumentChunkResponse(
                        101L,
                        1L,
                        0,
                        "cache key chunk content",
                        21,
                        "RECURSIVE_TEXT",
                        0,
                        21,
                        "heading",
                        LocalDateTime.of(2026, 7, 2, 10, 0)
                )
        ));

        mockMvc.perform(get("/documents/1/chunks"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$[0].id").value(101))
                .andExpect(jsonPath("$[0].chunkIndex").value(0))
                .andExpect(jsonPath("$[0].content").value("cache key chunk content"));
    }

    @Test
    void getDocumentChunksShouldReturnNotFoundForMissingDocument() throws Exception {
        when(documentService.getDocumentChunks(99L))
                .thenThrow(com.tuoman.ai_task_orchestrator.common.error.BusinessException.documentNotFound());

        mockMvc.perform(get("/documents/99/chunks"))
                .andExpect(status().isNotFound())
                .andExpect(jsonPath("$.code").value("DOCUMENT_NOT_FOUND"));
    }
}
