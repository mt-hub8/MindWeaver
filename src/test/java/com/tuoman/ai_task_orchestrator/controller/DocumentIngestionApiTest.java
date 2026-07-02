package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.common.error.GlobalExceptionHandler;
import com.tuoman.ai_task_orchestrator.dto.DocumentIngestionEventResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentIngestionEventTimelineResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentIngestionTaskResponse;
import com.tuoman.ai_task_orchestrator.service.DocumentIngestionEventService;
import com.tuoman.ai_task_orchestrator.service.DocumentIngestionTaskService;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.context.annotation.Import;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.test.web.servlet.MockMvc;

import java.time.LocalDateTime;
import java.util.List;
import java.util.Map;

import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@WebMvcTest(controllers = DocumentIngestionController.class)
@Import(GlobalExceptionHandler.class)
class DocumentIngestionApiTest {

    @Autowired
    private MockMvc mockMvc;

    @MockitoBean
    private DocumentIngestionTaskService documentIngestionTaskService;

    @MockitoBean
    private DocumentIngestionEventService documentIngestionEventService;

    @Test
    void getTaskShouldReturnTaskDetail() throws Exception {
        when(documentIngestionTaskService.getTask(1001L)).thenReturn(sampleTask("PROCESSING", "EMBEDDING"));

        mockMvc.perform(get("/documents/ingestions/1001"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.taskId").value(1001))
                .andExpect(jsonPath("$.displayStatus").value("处理中"))
                .andExpect(jsonPath("$.displayStep").value("正在生成文档向量"));
    }

    @Test
    void listTasksShouldReturnRecentTasks() throws Exception {
        when(documentIngestionTaskService.listRecentTasks()).thenReturn(List.of(sampleTask("PENDING", "TEXT_EXTRACTED")));

        mockMvc.perform(get("/documents/ingestions"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$[0].taskId").value(1001))
                .andExpect(jsonPath("$[0].displayStatus").value("待处理"));
    }

    @Test
    void getEventsShouldReturnTimeline() throws Exception {
        when(documentIngestionEventService.getEventTimeline(1001L)).thenReturn(new DocumentIngestionEventTimelineResponse(
                1001L,
                List.of(new DocumentIngestionEventResponse(
                        1L,
                        "TASK_CREATED",
                        "UPLOADED",
                        "COMPLETED",
                        "文档已提交，系统已创建处理任务。",
                        "Task created",
                        null,
                        Map.of("filename", "demo.txt"),
                        null,
                        null,
                        null,
                        LocalDateTime.of(2026, 7, 2, 10, 1, 21)
                ))
        ));

        mockMvc.perform(get("/documents/ingestions/1001/events"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.taskId").value(1001))
                .andExpect(jsonPath("$.events[0].eventType").value("TASK_CREATED"))
                .andExpect(jsonPath("$.events[0].displayMessage").value("文档已提交，系统已创建处理任务。"));
    }

    @Test
    void getEventsShouldReturnNotFoundWhenTaskMissing() throws Exception {
        when(documentIngestionEventService.getEventTimeline(999L))
                .thenThrow(BusinessException.ingestionTaskNotFound());

        mockMvc.perform(get("/documents/ingestions/999/events"))
                .andExpect(status().isNotFound())
                .andExpect(jsonPath("$.code").value("INGESTION_TASK_NOT_FOUND"));
    }

    @Test
    void retryShouldReturnPendingTask() throws Exception {
        when(documentIngestionTaskService.retryTask(1001L)).thenReturn(sampleTask("PENDING", "TEXT_EXTRACTED"));

        mockMvc.perform(post("/documents/ingestions/1001/retry"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.status").value("PENDING"));
    }

    @Test
    void retryShouldRejectNonFailedTask() throws Exception {
        when(documentIngestionTaskService.retryTask(1001L))
                .thenThrow(BusinessException.ingestionRetryNotAllowed("只有失败状态的任务可以重新处理"));

        mockMvc.perform(post("/documents/ingestions/1001/retry"))
                .andExpect(status().isConflict())
                .andExpect(jsonPath("$.code").value("INGESTION_RETRY_NOT_ALLOWED"));
    }

    private DocumentIngestionTaskResponse sampleTask(String status, String step) {
        String displayStatus = switch (status) {
            case "PENDING" -> "待处理";
            case "PROCESSING" -> "处理中";
            case "COMPLETED" -> "已完成";
            case "FAILED" -> "失败";
            default -> status;
        };
        String displayStep = switch (step) {
            case "EMBEDDING" -> "正在生成文档向量";
            case "TEXT_EXTRACTED" -> "已读取文档文本";
            default -> step;
        };
        return new DocumentIngestionTaskResponse(
                1001L,
                42L,
                "demo.txt",
                status,
                displayStatus,
                step,
                displayStep,
                "请稍候",
                2,
                1,
                0,
                null,
                null,
                0,
                LocalDateTime.now(),
                LocalDateTime.now(),
                null,
                "文档已进入处理队列，请稍候。",
                LocalDateTime.now()
        );
    }
}
