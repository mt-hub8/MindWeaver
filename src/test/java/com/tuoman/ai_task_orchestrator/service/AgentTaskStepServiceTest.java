package com.tuoman.ai_task_orchestrator.service;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.tuoman.ai_task_orchestrator.agent.AgentWorkflowJsonCodec;
import com.tuoman.ai_task_orchestrator.agent.tool.AgentToolNames;
import com.tuoman.ai_task_orchestrator.entity.AgentTaskStepEntity;
import com.tuoman.ai_task_orchestrator.enums.AgentTaskStepStatus;
import com.tuoman.ai_task_orchestrator.enums.AgentTaskStepType;
import com.tuoman.ai_task_orchestrator.repository.AgentTaskStepRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.ArgumentCaptor;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class AgentTaskStepServiceTest {

    @Mock
    private AgentTaskStepRepository agentTaskStepRepository;

    private AgentTaskStepService agentTaskStepService;

    @BeforeEach
    void setUp() {
        agentTaskStepService = new AgentTaskStepService(
                agentTaskStepRepository,
                new AgentWorkflowJsonCodec(new ObjectMapper())
        );
    }

    @Test
    void createFixedPlanShouldCreateThreeChineseSteps() {
        when(agentTaskStepRepository.existsByTaskId(1L)).thenReturn(false);
        when(agentTaskStepRepository.save(any(AgentTaskStepEntity.class))).thenAnswer(invocation -> {
            AgentTaskStepEntity step = invocation.getArgument(0);
            step.setId((long) step.getStepOrder());
            return step;
        });
        when(agentTaskStepRepository.findByTaskIdOrderByStepOrderAsc(1L)).thenAnswer(invocation -> List.of());

        agentTaskStepService.createFixedPlan(1L);

        ArgumentCaptor<AgentTaskStepEntity> captor = ArgumentCaptor.forClass(AgentTaskStepEntity.class);
        verify(agentTaskStepRepository, org.mockito.Mockito.times(3)).save(captor.capture());
        List<AgentTaskStepEntity> saved = captor.getAllValues();

        assertThat(saved.get(0).getStepOrder()).isEqualTo(1);
        assertThat(saved.get(0).getStepType()).isEqualTo(AgentTaskStepType.TOOL_CALL);
        assertThat(saved.get(0).getToolName()).isEqualTo(AgentToolNames.KNOWLEDGE_SEARCH);
        assertThat(saved.get(0).getDisplayTitle()).isEqualTo("检索知识库");
        assertThat(saved.get(0).getStatus()).isEqualTo(AgentTaskStepStatus.PENDING);

        assertThat(saved.get(1).getToolName()).isEqualTo(AgentToolNames.CONTEXT_SUMMARY);
        assertThat(saved.get(1).getDisplayTitle()).isEqualTo("总结检索结果");

        assertThat(saved.get(2).getStepType()).isEqualTo(AgentTaskStepType.FINAL_REPORT);
        assertThat(saved.get(2).getDisplayTitle()).isEqualTo("生成最终报告");
    }
}
