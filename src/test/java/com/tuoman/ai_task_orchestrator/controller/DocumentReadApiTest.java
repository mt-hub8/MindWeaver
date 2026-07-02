package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.common.error.GlobalExceptionHandler;
import com.tuoman.ai_task_orchestrator.dto.CollectionMembershipResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentDeleteResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentChunkResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentReindexSubmitResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentSummaryResponse;
import com.tuoman.ai_task_orchestrator.service.DocumentEmbeddingService;
import com.tuoman.ai_task_orchestrator.service.DocumentIngestionService;
import com.tuoman.ai_task_orchestrator.service.DocumentReindexService;
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
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.delete;
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

    @MockitoBean
    private DocumentReindexService documentReindexService;

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
                new DocumentSummaryResponse(
                        1L,
                        "demo.md",
                        3,
                        "ACTIVE",
                        "已启用",
                        "READY",
                        "索引就绪",
                        "当前文档可以用于知识库问答。",
                        null,
                        true,
                        true,
                        1,
                        0,
                        null,
                        true,
                        null,
                        List.of(new CollectionMembershipResponse(1L, "项目 A")),
                        List.of(1L),
                        List.of("项目 A"),
                        true,
                        true,
                        createdAt,
                        createdAt
                )
        ));

        mockMvc.perform(get("/documents"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$[0].documentId").value(1))
                .andExpect(jsonPath("$[0].title").value("demo.md"))
                .andExpect(jsonPath("$[0].chunkCount").value(3))
                .andExpect(jsonPath("$[0].status").value("ACTIVE"))
                .andExpect(jsonPath("$[0].displayStatus").value("已启用"))
                .andExpect(jsonPath("$[0].processingStatus").value("READY"))
                .andExpect(jsonPath("$[0].displayProcessingStatus").value("索引就绪"))
                .andExpect(jsonPath("$[0].lifecycleHint").value("当前文档可以用于知识库问答。"))
                .andExpect(jsonPath("$[0].canDelete").value(true))
                .andExpect(jsonPath("$[0].canAsk").value(true))
                .andExpect(jsonPath("$[0].currentGeneration").value(1))
                .andExpect(jsonPath("$[0].canReindex").value(true))
                .andExpect(jsonPath("$[0].collectionNames[0]").value("项目 A"))
                .andExpect(jsonPath("$[0].canAssignToCollection").value(true))
                .andExpect(jsonPath("$[0].canRemoveFromCollection").value(true));
    }

    @Test
    void reindexDocumentShouldReturnAcceptedResponse() throws Exception {
        when(documentReindexService.submitReindex(1L)).thenReturn(new DocumentReindexSubmitResponse(
                2001L,
                1L,
                "demo.md",
                "PENDING",
                "待处理",
                "已提交重新索引任务，请在处理记录中查看进度。"
        ));

        mockMvc.perform(org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post("/documents/1/reindex"))
                .andExpect(status().isAccepted())
                .andExpect(jsonPath("$.taskId").value(2001))
                .andExpect(jsonPath("$.displayMessage").value("已提交重新索引任务，请在处理记录中查看进度。"));
    }

    @Test
    void deleteDocumentShouldReturnLifecycleResponse() throws Exception {
        LocalDateTime deletedAt = LocalDateTime.of(2026, 7, 2, 10, 30);
        when(documentService.softDeleteDocument(1L)).thenReturn(new DocumentDeleteResponse(
                1L,
                "DELETED",
                "已删除",
                "删除成功：该文档不会再用于知识库问答。",
                deletedAt
        ));

        mockMvc.perform(delete("/documents/1"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.documentId").value(1))
                .andExpect(jsonPath("$.status").value("DELETED"))
                .andExpect(jsonPath("$.displayStatus").value("已删除"))
                .andExpect(jsonPath("$.message").value("删除成功：该文档不会再用于知识库问答。"));
    }

    @Test
    void deleteDocumentShouldReturnNotFoundForMissingDocument() throws Exception {
        when(documentService.softDeleteDocument(99L))
                .thenThrow(com.tuoman.ai_task_orchestrator.common.error.BusinessException.documentNotFound());

        mockMvc.perform(delete("/documents/99"))
                .andExpect(status().isNotFound())
                .andExpect(jsonPath("$.code").value("DOCUMENT_NOT_FOUND"));
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
