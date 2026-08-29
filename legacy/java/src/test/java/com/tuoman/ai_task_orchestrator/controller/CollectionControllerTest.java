package com.tuoman.ai_task_orchestrator.controller;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.tuoman.ai_task_orchestrator.common.error.GlobalExceptionHandler;
import com.tuoman.ai_task_orchestrator.dto.CollectionAssignmentResponse;
import com.tuoman.ai_task_orchestrator.dto.CollectionDetailResponse;
import com.tuoman.ai_task_orchestrator.dto.CollectionDocumentItemResponse;
import com.tuoman.ai_task_orchestrator.dto.CollectionSummaryResponse;
import com.tuoman.ai_task_orchestrator.dto.CreateCollectionRequest;
import com.tuoman.ai_task_orchestrator.service.CollectionService;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.context.annotation.Import;
import org.springframework.http.MediaType;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.test.web.servlet.MockMvc;

import java.time.LocalDateTime;
import java.util.List;

import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.delete;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@WebMvcTest(controllers = CollectionController.class)
@Import(GlobalExceptionHandler.class)
class CollectionControllerTest {

    @Autowired
    private MockMvc mockMvc;

    @Autowired
    private ObjectMapper objectMapper;

    @MockitoBean
    private CollectionService collectionService;

    @Test
    void createCollectionShouldReturnCreatedSummary() throws Exception {
        LocalDateTime now = LocalDateTime.of(2026, 7, 2, 12, 0);
        when(collectionService.createCollection(org.mockito.ArgumentMatchers.any())).thenReturn(
                new CollectionSummaryResponse(1L, "项目 A", "说明", 0, 0, now, now)
        );

        CreateCollectionRequest request = new CreateCollectionRequest();
        request.setName("项目 A");
        request.setDescription("说明");

        mockMvc.perform(post("/collections")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(objectMapper.writeValueAsString(request)))
                .andExpect(status().isCreated())
                .andExpect(jsonPath("$.collectionId").value(1))
                .andExpect(jsonPath("$.name").value("项目 A"))
                .andExpect(jsonPath("$.documentCount").value(0))
                .andExpect(jsonPath("$.activeDocumentCount").value(0));
    }

    @Test
    void listCollectionsShouldReturnSummaries() throws Exception {
        LocalDateTime now = LocalDateTime.of(2026, 7, 2, 12, 0);
        when(collectionService.listCollections()).thenReturn(List.of(
                new CollectionSummaryResponse(1L, "分组 A", null, 2, 1, now, now)
        ));

        mockMvc.perform(get("/collections"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$[0].collectionId").value(1))
                .andExpect(jsonPath("$[0].activeDocumentCount").value(1));
    }

    @Test
    void getCollectionShouldReturnDetail() throws Exception {
        LocalDateTime now = LocalDateTime.of(2026, 7, 2, 12, 0);
        when(collectionService.getCollection(1L)).thenReturn(new CollectionDetailResponse(
                1L,
                "分组 A",
                "说明",
                1,
                1,
                now,
                now,
                List.of(new CollectionDocumentItemResponse(
                        10L,
                        "demo.md",
                        "ACTIVE",
                        "已启用",
                        true,
                        true,
                        "当前文档可以用于知识库问答。"
                ))
        ));

        mockMvc.perform(get("/collections/1"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.collectionId").value(1))
                .andExpect(jsonPath("$.documents[0].documentId").value(10))
                .andExpect(jsonPath("$.documents[0].canAsk").value(true));
    }

    @Test
    void assignDocumentShouldReturnAssignmentResponse() throws Exception {
        when(collectionService.assignDocument(1L, 10L))
                .thenReturn(new CollectionAssignmentResponse(1L, 10L, "文档已加入分组"));

        mockMvc.perform(post("/collections/1/documents/10"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.message").value("文档已加入分组"));
    }

    @Test
    void removeDocumentShouldReturnAssignmentResponse() throws Exception {
        when(collectionService.removeDocument(eq(1L), eq(10L)))
                .thenReturn(new CollectionAssignmentResponse(1L, 10L, "该文档已从分组移出"));

        mockMvc.perform(delete("/collections/1/documents/10"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.message").value("该文档已从分组移出"));
    }
}
