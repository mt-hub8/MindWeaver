package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.agent.AgentTaskEventRecorder;
import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.common.error.ErrorCode;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingProvider;
import com.tuoman.ai_task_orchestrator.embedding.MockEmbeddingClient;
import com.tuoman.ai_task_orchestrator.entity.AgentTaskEntity;
import com.tuoman.ai_task_orchestrator.enums.AgentTaskStatus;
import com.tuoman.ai_task_orchestrator.repository.AgentTaskRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.Optional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.doThrow;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;
import static org.mockito.Mockito.eq;

@ExtendWith(MockitoExtension.class)
class AgentTaskExecutorTest {

    @Mock
    private AgentTaskRepository agentTaskRepository;

    @Mock
    private AgentTaskEventRecorder agentTaskEventRecorder;

    @Mock
    private AgentTaskWorkflowService agentTaskWorkflowService;

    @Mock
    private EmbeddingProvider embeddingProvider;

    private AgentTaskExecutor agentTaskExecutor;

    @BeforeEach
    void setUp() {
        agentTaskExecutor = new AgentTaskExecutor(
                agentTaskRepository,
                agentTaskEventRecorder,
                agentTaskWorkflowService,
                embeddingProvider
        );
        when(embeddingProvider.provider()).thenReturn(MockEmbeddingClient.PROVIDER);
        when(embeddingProvider.model()).thenReturn(MockEmbeddingClient.DEFAULT_MODEL);
    }

    @Test
    void executeShouldDelegateToWorkflowService() {
        AgentTaskEntity task = pendingTask(1L);
        when(agentTaskRepository.findById(1L)).thenReturn(Optional.of(task));
        when(agentTaskRepository.save(any(AgentTaskEntity.class))).thenAnswer(invocation -> invocation.getArgument(0));

        agentTaskExecutor.execute(1L);

        verify(agentTaskEventRecorder).recordTaskStarted(1L);
        verify(agentTaskWorkflowService).executeWorkflow(task);
    }

    @Test
    void executeShouldMarkFailedWhenWorkflowThrows() {
        AgentTaskEntity task = pendingTask(2L);
        when(agentTaskRepository.findById(2L)).thenReturn(Optional.of(task));
        when(agentTaskRepository.save(any(AgentTaskEntity.class))).thenAnswer(invocation -> invocation.getArgument(0));
        doThrow(BusinessException.agentFinalReportFailed("llm failed")).when(agentTaskWorkflowService).executeWorkflow(task);

        assertThatThrownBy(() -> agentTaskExecutor.execute(2L))
                .isInstanceOf(BusinessException.class)
                .extracting("errorCode")
                .isEqualTo(ErrorCode.AGENT_FINAL_REPORT_FAILED);

        assertThat(task.getStatus()).isEqualTo(AgentTaskStatus.FAILED);
        verify(agentTaskEventRecorder).recordTaskFailed(eq(2L), any(), any(), any(), any(), any());
    }

    private AgentTaskEntity pendingTask(Long id) {
        AgentTaskEntity task = new AgentTaskEntity();
        task.setId(id);
        task.setTitle("任务");
        task.setObjective("分析目标");
        task.setStatus(AgentTaskStatus.PENDING);
        return task;
    }
}
