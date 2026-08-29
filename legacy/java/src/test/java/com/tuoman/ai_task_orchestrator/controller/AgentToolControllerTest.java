package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.common.error.GlobalExceptionHandler;
import com.tuoman.ai_task_orchestrator.dto.AgentToolResponse;
import com.tuoman.ai_task_orchestrator.service.AgentToolQueryService;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.context.annotation.Import;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.test.web.servlet.MockMvc;

import java.util.List;
import java.util.Map;

import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@WebMvcTest(controllers = AgentToolController.class)
@Import(GlobalExceptionHandler.class)
class AgentToolControllerTest {

    @Autowired
    private MockMvc mockMvc;

    @MockitoBean
    private AgentToolQueryService agentToolQueryService;

    @Test
    void listToolsShouldReturnChineseDisplayNames() throws Exception {
        when(agentToolQueryService.listTools()).thenReturn(List.of(
                new AgentToolResponse(
                        "knowledge_search",
                        "检索知识库",
                        "检索相关片段",
                        Map.of("query", "string"),
                        Map.of("citations", "list"),
                        true
                ),
                new AgentToolResponse(
                        "context_summary",
                        "总结检索结果",
                        "总结片段",
                        Map.of(),
                        Map.of(),
                        true
                )
        ));

        mockMvc.perform(get("/agent/tools"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$[0].toolName").value("knowledge_search"))
                .andExpect(jsonPath("$[0].displayName").value("检索知识库"))
                .andExpect(jsonPath("$[1].toolName").value("context_summary"))
                .andExpect(jsonPath("$[1].displayName").value("总结检索结果"));
    }
}
