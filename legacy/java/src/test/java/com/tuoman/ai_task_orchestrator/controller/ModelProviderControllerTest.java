package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.common.error.GlobalExceptionHandler;
import com.tuoman.ai_task_orchestrator.dto.ModelProviderConfigRequest;
import com.tuoman.ai_task_orchestrator.dto.ModelProviderConfigResponse;
import com.tuoman.ai_task_orchestrator.dto.ModelProviderTestResultResponse;
import com.tuoman.ai_task_orchestrator.modelprovider.ModelProviderTestStatus;
import com.tuoman.ai_task_orchestrator.modelprovider.ModelProviderType;
import com.tuoman.ai_task_orchestrator.service.ModelProviderConfigService;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.context.annotation.Import;
import org.springframework.http.MediaType;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.test.web.servlet.MockMvc;

import java.time.LocalDateTime;
import java.util.List;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.put;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@WebMvcTest(controllers = ModelProviderController.class)
@Import(GlobalExceptionHandler.class)
class ModelProviderControllerTest {

    @Autowired
    private MockMvc mockMvc;

    @MockitoBean
    private ModelProviderConfigService modelProviderConfigService;

    @Test
    void listShouldNotExposePlainApiKey() throws Exception {
        when(modelProviderConfigService.listAll()).thenReturn(List.of(sampleResponse()));

        mockMvc.perform(get("/model-providers"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$[0].apiKeyMasked").value("sk-****1234"))
                .andExpect(jsonPath("$[0].apiKey").doesNotExist());
    }

    @Test
    void createShouldAcceptProvider() throws Exception {
        when(modelProviderConfigService.create(any())).thenReturn(sampleResponse());

        mockMvc.perform(post("/model-providers")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("""
                                {
                                  "providerType": "MOCK",
                                  "displayName": "测试 Mock"
                                }
                                """))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.displayName").value("测试 Mock"));
    }

    @Test
    void testMockProviderShouldReturnSuccess() throws Exception {
        when(modelProviderConfigService.testConnection(1L)).thenReturn(
                new ModelProviderTestResultResponse(
                        ModelProviderTestStatus.SUCCESS,
                        "Mock 供应商连接正常",
                        ModelProviderType.MOCK,
                        LocalDateTime.now(),
                        5L
                )
        );

        mockMvc.perform(post("/model-providers/1/test"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.status").value("SUCCESS"));
    }

    @Test
    void setDefaultLlmShouldDelegate() throws Exception {
        when(modelProviderConfigService.setDefaultLlm(2L)).thenReturn(sampleResponse());

        mockMvc.perform(post("/model-providers/2/set-default-llm"))
                .andExpect(status().isOk());

        verify(modelProviderConfigService).setDefaultLlm(2L);
    }

    @Test
    void setDefaultEmbeddingShouldDelegate() throws Exception {
        when(modelProviderConfigService.setDefaultEmbedding(3L)).thenReturn(sampleResponse());

        mockMvc.perform(post("/model-providers/3/set-default-embedding"))
                .andExpect(status().isOk());

        verify(modelProviderConfigService).setDefaultEmbedding(3L);
    }

    @Test
    void updateShouldDelegate() throws Exception {
        when(modelProviderConfigService.update(eq(9L), any())).thenReturn(sampleResponse());

        mockMvc.perform(put("/model-providers/9")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("""
                                {
                                  "providerType": "MOCK",
                                  "displayName": "更新"
                                }
                                """))
                .andExpect(status().isOk());

        verify(modelProviderConfigService).update(eq(9L), any());
    }

    private ModelProviderConfigResponse sampleResponse() {
        return new ModelProviderConfigResponse(
                1L,
                ModelProviderType.MOCK,
                "测试 Mock",
                null,
                "sk-****1234",
                "mock-llm",
                "mock-embedding-v1",
                null,
                128,
                true,
                false,
                false,
                ModelProviderTestStatus.SUCCESS,
                "ok",
                LocalDateTime.now(),
                LocalDateTime.now(),
                LocalDateTime.now()
        );
    }
}
