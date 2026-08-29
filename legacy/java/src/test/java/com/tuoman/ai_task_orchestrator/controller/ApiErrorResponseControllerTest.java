package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.common.error.GlobalExceptionHandler;
import com.tuoman.ai_task_orchestrator.service.DocumentEmbeddingService;
import com.tuoman.ai_task_orchestrator.service.DocumentIngestionService;
import com.tuoman.ai_task_orchestrator.service.DocumentReindexService;
import com.tuoman.ai_task_orchestrator.service.DocumentService;
import com.tuoman.ai_task_orchestrator.service.TaskAttemptService;
import com.tuoman.ai_task_orchestrator.service.TaskOutputChunkService;
import com.tuoman.ai_task_orchestrator.service.TaskService;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.context.annotation.Import;
import org.springframework.http.MediaType;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.test.web.servlet.MockMvc;

import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@WebMvcTest(controllers = {TaskController.class, DocumentController.class})
@Import(GlobalExceptionHandler.class)
class ApiErrorResponseControllerTest {

    @Autowired
    private MockMvc mockMvc;

    @MockitoBean
    private TaskService taskService;

    @MockitoBean
    private TaskOutputChunkService taskOutputChunkService;

    @MockitoBean
    private TaskAttemptService taskAttemptService;

    @MockitoBean
    private DocumentService documentService;

    @MockitoBean
    private DocumentEmbeddingService documentEmbeddingService;

    @MockitoBean
    private DocumentIngestionService documentIngestionService;

    @MockitoBean
    private DocumentReindexService documentReindexService;

    @Test
    void getMissingTaskShouldReturnTaskNotFoundErrorResponse() throws Exception {
        when(taskService.getTaskById(999L)).thenThrow(BusinessException.taskNotFound());

        mockMvc.perform(get("/tasks/999"))
                .andExpect(status().isNotFound())
                .andExpect(jsonPath("$.status").value(404))
                .andExpect(jsonPath("$.code").value("TASK_NOT_FOUND"))
                .andExpect(jsonPath("$.message").value("任务不存在"))
                .andExpect(jsonPath("$.path").value("/tasks/999"))
                .andExpect(jsonPath("$.timestamp").isNotEmpty())
                .andExpect(jsonPath("$.traceId").isNotEmpty());
    }

    @Test
    void createTaskWithInvalidBodyShouldReturnValidationErrorResponse() throws Exception {
        mockMvc.perform(post("/tasks")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{}"))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.status").value(400))
                .andExpect(jsonPath("$.code").value("VALIDATION_ERROR"))
                .andExpect(jsonPath("$.message").value("prompt不能为空"))
                .andExpect(jsonPath("$.path").value("/tasks"))
                .andExpect(jsonPath("$.traceId").isNotEmpty());
    }

    @Test
    void createTaskWithUnreadableJsonShouldReturnInvalidRequest() throws Exception {
        mockMvc.perform(post("/tasks")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("not-json"))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("INVALID_REQUEST"))
                .andExpect(jsonPath("$.path").value("/tasks"));
    }

    @Test
    void getMissingDocumentShouldReturnDocumentNotFoundErrorResponse() throws Exception {
        when(documentService.getDocument(42L)).thenThrow(BusinessException.documentNotFound());

        mockMvc.perform(get("/documents/42"))
                .andExpect(status().isNotFound())
                .andExpect(jsonPath("$.status").value(404))
                .andExpect(jsonPath("$.code").value("DOCUMENT_NOT_FOUND"))
                .andExpect(jsonPath("$.message").value("Document not found"))
                .andExpect(jsonPath("$.path").value("/documents/42"));
    }

    @Test
    void cancelTaskWithInvalidStatusShouldReturnInvalidTaskStatus() throws Exception {
        when(taskService.cancelTask(eq(7L), eq("任务已取消")))
                .thenThrow(BusinessException.invalidTaskStatus("当前任务状态不允许取消"));

        mockMvc.perform(post("/tasks/7/cancel"))
                .andExpect(status().isConflict())
                .andExpect(jsonPath("$.status").value(409))
                .andExpect(jsonPath("$.code").value("INVALID_TASK_STATUS"))
                .andExpect(jsonPath("$.message").value("当前任务状态不允许取消"))
                .andExpect(jsonPath("$.path").value("/tasks/7/cancel"));
    }
}
