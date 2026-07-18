package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.agent.AgentTaskEventRecorder;
import com.tuoman.ai_task_orchestrator.dto.MemoryResponse;
import com.tuoman.ai_task_orchestrator.dto.SaveAgentTaskMemoryRequest;
import com.tuoman.ai_task_orchestrator.entity.AgentTaskEntity;
import com.tuoman.ai_task_orchestrator.enums.AgentTaskStatus;
import com.tuoman.ai_task_orchestrator.enums.MemoryScope;
import com.tuoman.ai_task_orchestrator.enums.MemorySourceType;
import com.tuoman.ai_task_orchestrator.enums.MemoryType;
import com.tuoman.ai_task_orchestrator.mq.AgentTaskMessagePublisher;
import com.tuoman.ai_task_orchestrator.repository.AgentTaskRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.ArgumentCaptor;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;
import org.springframework.test.util.ReflectionTestUtils;

import java.util.Optional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class AgentTaskMemoryIntegrationTest {

    @Mock
    private AgentTaskRepository agentTaskRepository;
    @Mock
    private CollectionService collectionService;
    @Mock
    private AgentTaskMessagePublisher publisher;
    @Mock
    private AgentTaskEventRecorder eventRecorder;
    @Mock
    private AgentTaskStepService stepService;
    @Mock
    private MemoryService memoryService;

    private AgentTaskService service;

    @BeforeEach
    void setUp() {
        service = new AgentTaskService(agentTaskRepository, collectionService, publisher, eventRecorder, stepService);
        ReflectionTestUtils.setField(service, "memoryService", memoryService);
    }

    @Test
    void completedTaskSummaryShouldOnlySaveAfterExplicitConfirmation() {
        AgentTaskEntity task = new AgentTaskEntity();
        task.setId(77L);
        task.setTitle("V19 实现");
        task.setResult("已完成 Memory Foundation");
        task.setStatus(AgentTaskStatus.COMPLETED);
        task.setAgentProfileId(3L);
        when(agentTaskRepository.findById(77L)).thenReturn(Optional.of(task));
        when(memoryService.createMemory(any())).thenReturn(MemoryResponse.builder().id(99L).build());

        SaveAgentTaskMemoryRequest request = new SaveAgentTaskMemoryRequest();
        request.setConfirmed(true);
        MemoryResponse response = service.saveSummaryAsMemory(77L, request);

        assertThat(response.getId()).isEqualTo(99L);
        ArgumentCaptor<com.tuoman.ai_task_orchestrator.dto.MemoryRequest> captor =
                ArgumentCaptor.forClass(com.tuoman.ai_task_orchestrator.dto.MemoryRequest.class);
        verify(memoryService).createMemory(captor.capture());
        assertThat(captor.getValue().getMemoryType()).isEqualTo(MemoryType.TASK_RESULT);
        assertThat(captor.getValue().getMemoryScope()).isEqualTo(MemoryScope.TASK);
        assertThat(captor.getValue().getSourceType()).isEqualTo(MemorySourceType.AGENT_TASK);
        assertThat(captor.getValue().getTaskId()).isEqualTo(77L);
        assertThat(captor.getValue().getAgentProfileId()).isEqualTo(3L);
    }

    @Test
    void taskSummaryShouldNotAutoSaveWithoutConfirmation() {
        SaveAgentTaskMemoryRequest request = new SaveAgentTaskMemoryRequest();
        request.setConfirmed(false);

        assertThatThrownBy(() -> service.saveSummaryAsMemory(77L, request))
                .hasMessageContaining("用户确认");
        verify(memoryService, never()).createMemory(any());
    }
}
