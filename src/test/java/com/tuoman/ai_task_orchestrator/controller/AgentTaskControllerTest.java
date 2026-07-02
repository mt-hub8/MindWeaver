package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.common.error.GlobalExceptionHandler;
import com.tuoman.ai_task_orchestrator.dto.AgentTaskDetailResponse;
import com.tuoman.ai_task_orchestrator.dto.AgentTaskEventResponse;
import com.tuoman.ai_task_orchestrator.dto.AgentTaskModelMetadataResponse;
import com.tuoman.ai_task_orchestrator.dto.AgentTaskSummaryResponse;
import com.tuoman.ai_task_orchestrator.dto.CreateAgentTaskResponse;
import com.tuoman.ai_task_orchestrator.service.AgentTaskQueryService;
import com.tuoman.ai_task_orchestrator.service.AgentTaskService;
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
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@WebMvcTest(controllers = AgentTaskController.class)
@Import(GlobalExceptionHandler.class)
class AgentTaskControllerTest {

    @Autowired
    private MockMvc mockMvc;

    @MockitoBean
    private AgentTaskService agentTaskService;

    @MockitoBean
    private AgentTaskQueryService agentTaskQueryService;

    @Test
    void createTaskShouldReturnAccepted() throws Exception {
        when(agentTaskService.createTask(org.mockito.ArgumentMatchers.any())).thenReturn(
                new CreateAgentTaskResponse(1001L, "PENDING", "待处理", "AI 任务已创建，系统将在后台检索知识库并生成结果。")
        );

        mockMvc.perform(post("/agent/tasks")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("""
                                {
                                  "title": "总结项目 A",
                                  "objective": "总结核心功能",
                                  "collectionId": 1
                                }
                                """))
                .andExpect(status().isAccepted())
                .andExpect(jsonPath("$.taskId").value(1001))
                .andExpect(jsonPath("$.displayStatus").value("待处理"));
    }

    @Test
    void listTasksShouldReturnSummaries() throws Exception {
        when(agentTaskQueryService.listTasks()).thenReturn(List.of(
                new AgentTaskSummaryResponse(
                        1L,
                        "总结",
                        "COMPLETED",
                        "已完成",
                        2L,
                        "项目 A",
                        3,
                        LocalDateTime.of(2026, 7, 2, 10, 0),
                        LocalDateTime.of(2026, 7, 2, 10, 5)
                )
        ));

        mockMvc.perform(get("/agent/tasks"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$[0].taskId").value(1))
                .andExpect(jsonPath("$[0].displayStatus").value("已完成"));
    }

    @Test
    void getTaskShouldReturnDetail() throws Exception {
        when(agentTaskQueryService.getTask(1L)).thenReturn(new AgentTaskDetailResponse(
                1L,
                "总结",
                "目标",
                "COMPLETED",
                "已完成",
                null,
                null,
                "全部文档",
                "任务结果",
                null,
                null,
                null,
                new AgentTaskModelMetadataResponse("mock", "mock-llm", "mock", "mock-embedding-v1", 10, 20, 2, 1),
                List.of(),
                LocalDateTime.of(2026, 7, 2, 10, 0),
                LocalDateTime.of(2026, 7, 2, 10, 1),
                LocalDateTime.of(2026, 7, 2, 10, 5)
        ));

        mockMvc.perform(get("/agent/tasks/1"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.taskId").value(1))
                .andExpect(jsonPath("$.scopeLabel").value("全部文档"));
    }

    @Test
    void listEventsShouldReturnTimeline() throws Exception {
        when(agentTaskQueryService.listEvents(eq(1L))).thenReturn(List.of(
                new AgentTaskEventResponse(
                        10L,
                        "TASK_CREATED",
                        "任务已创建",
                        "CREATED",
                        "COMPLETED",
                        "已完成",
                        "Task created",
                        "任务已创建，等待后台执行。",
                        null,
                        null,
                        null,
                        null,
                        LocalDateTime.of(2026, 7, 2, 10, 0)
                )
        ));

        mockMvc.perform(get("/agent/tasks/1/events"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$[0].displayEventType").value("任务已创建"));
    }
}
