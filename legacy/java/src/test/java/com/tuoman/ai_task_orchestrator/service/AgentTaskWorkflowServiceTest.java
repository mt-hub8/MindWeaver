package com.tuoman.ai_task_orchestrator.service;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.tuoman.ai_task_orchestrator.agent.AgentTaskEventRecorder;
import com.tuoman.ai_task_orchestrator.agent.AgentTaskPromptBuilder;
import com.tuoman.ai_task_orchestrator.agent.AgentTaskProperties;
import com.tuoman.ai_task_orchestrator.agent.AgentWorkflowJsonCodec;
import com.tuoman.ai_task_orchestrator.agent.tool.AgentToolNames;
import com.tuoman.ai_task_orchestrator.agent.tool.AgentToolRegistry;
import com.tuoman.ai_task_orchestrator.agent.tool.ContextSummaryTool;
import com.tuoman.ai_task_orchestrator.agent.tool.KnowledgeSearchTool;
import com.tuoman.ai_task_orchestrator.agent.tool.ToolExecutionResult;
import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.common.error.ErrorCode;
import com.tuoman.ai_task_orchestrator.entity.AgentTaskEntity;
import com.tuoman.ai_task_orchestrator.entity.AgentTaskStepEntity;
import com.tuoman.ai_task_orchestrator.enums.AgentTaskStatus;
import com.tuoman.ai_task_orchestrator.enums.AgentTaskStepStatus;
import com.tuoman.ai_task_orchestrator.enums.AgentTaskStepType;
import com.tuoman.ai_task_orchestrator.llm.LlmGenerateOptions;
import com.tuoman.ai_task_orchestrator.llm.LlmGenerateResult;
import com.tuoman.ai_task_orchestrator.llm.LlmProvider;
import com.tuoman.ai_task_orchestrator.llm.MockLlmProvider;
import com.tuoman.ai_task_orchestrator.repository.AgentTaskCitationRepository;
import com.tuoman.ai_task_orchestrator.repository.AgentTaskRepository;
import com.tuoman.ai_task_orchestrator.repository.AgentTaskStepRepository;
import com.tuoman.ai_task_orchestrator.retrieval.CollectionAskEmptyReason;
import com.tuoman.ai_task_orchestrator.retrieval.CollectionAskScope;
import com.tuoman.ai_task_orchestrator.retrieval.RetrievalScope;
import com.tuoman.ai_task_orchestrator.rerank.RagTwoStageRetrievalService;
import com.tuoman.ai_task_orchestrator.rerank.RagTwoStageRetrievalService.RagRetrievalOutcome;
import com.tuoman.ai_task_orchestrator.rerank.RagTwoStageRetrievalService.RagRetrievedChunk;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.List;
import java.util.Map;
import java.util.Set;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyLong;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.atLeastOnce;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class AgentTaskWorkflowServiceTest {

    @Mock
    private AgentTaskRepository agentTaskRepository;

    @Mock
    private AgentTaskCitationRepository agentTaskCitationRepository;

    @Mock
    private AgentTaskStepRepository agentTaskStepRepository;

    @Mock
    private CollectionScopeService collectionScopeService;

    @Mock
    private RagTwoStageRetrievalService ragTwoStageRetrievalService;

    @Mock
    private LlmProvider llmProvider;

    @Mock
    private AgentTaskEventRecorder agentTaskEventRecorder;

    private AgentTaskWorkflowService workflowService;

    @BeforeEach
    void setUp() {
        AgentTaskProperties properties = new AgentTaskProperties();
        properties.setDefaultTopK(5);

        KnowledgeSearchTool knowledgeSearchTool = new KnowledgeSearchTool(
                collectionScopeService,
                ragTwoStageRetrievalService,
                properties
        );
        ContextSummaryTool contextSummaryTool = new ContextSummaryTool(llmProvider);
        AgentToolRegistry registry = new AgentToolRegistry(List.of(knowledgeSearchTool, contextSummaryTool));

        AgentWorkflowJsonCodec jsonCodec = new AgentWorkflowJsonCodec(new ObjectMapper());
        AgentTaskStepService stepService = new AgentTaskStepService(agentTaskStepRepository, jsonCodec);

        workflowService = new AgentTaskWorkflowService(
                agentTaskRepository,
                agentTaskCitationRepository,
                stepService,
                registry,
                agentTaskEventRecorder,
                new AgentTaskPromptBuilder(),
                llmProvider,
                properties
        );

        when(agentTaskRepository.save(any(AgentTaskEntity.class))).thenAnswer(invocation -> invocation.getArgument(0));
        when(agentTaskStepRepository.save(any(AgentTaskStepEntity.class))).thenAnswer(invocation -> invocation.getArgument(0));
    }

    @Test
    void executeWorkflowShouldCompleteWithoutLlmWhenNoContext() {
        AgentTaskEntity task = task(1L, 9L);
        when(agentTaskStepRepository.findByTaskIdOrderByStepOrderAsc(1L)).thenReturn(steps(1L));
        when(collectionScopeService.resolveForAsk(9L)).thenReturn(new CollectionAskScope(
                9L,
                "空分组",
                CollectionAskEmptyReason.NO_DOCUMENTS,
                Set.of(),
                Set.of(),
                "当前分组下没有可用于问答的文档"
        ));

        workflowService.executeWorkflow(task);

        assertThat(task.getStatus()).isEqualTo(AgentTaskStatus.COMPLETED);
        assertThat(task.getResult()).contains("当前范围内没有可用于执行任务的知识库内容");
        verify(llmProvider, never()).generate(anyString(), anyString(), any(LlmGenerateOptions.class));
        verify(agentTaskEventRecorder).recordFinalReportCompleted(eq(1L), eq(0L), any());
    }

    @Test
    void executeWorkflowShouldCompleteWithLlmWhenContextExists() {
        AgentTaskEntity task = task(2L, null);
        when(agentTaskStepRepository.findByTaskIdOrderByStepOrderAsc(2L)).thenReturn(steps(2L));
        when(ragTwoStageRetrievalService.retrieve(anyString(), eq(5), any(RetrievalScope.class)))
                .thenReturn(new RagRetrievalOutcome(
                        List.of(new RagRetrievedChunk(1, 1, 10L, "doc", 20L, 0.9, null, "content")),
                        5,
                        5,
                        false,
                        null,
                        0L
                ));
        when(llmProvider.generate(anyString(), anyString(), any(LlmGenerateOptions.class)))
                .thenReturn(
                        LlmGenerateResult.success("摘要", MockLlmProvider.PROVIDER, MockLlmProvider.DEFAULT_MODEL, 5, 5, 1L, "stop", null),
                        LlmGenerateResult.success("## 任务结论\n完成", MockLlmProvider.PROVIDER, MockLlmProvider.DEFAULT_MODEL, 10, 20, 2L, "stop", null)
                );

        workflowService.executeWorkflow(task);

        assertThat(task.getStatus()).isEqualTo(AgentTaskStatus.COMPLETED);
        assertThat(task.getResult()).contains("任务结论");
        assertThat(task.getCitationCount()).isEqualTo(1);
        verify(agentTaskCitationRepository, atLeastOnce()).save(any());
        verify(agentTaskEventRecorder).recordToolExecutionCompleted(eq(2L), eq(AgentToolNames.KNOWLEDGE_SEARCH), anyString(), anyLong());
        verify(agentTaskEventRecorder).recordFinalReportCompleted(eq(2L), anyLong(), any());
    }

    @Test
    void executeWorkflowShouldFailWhenFinalReportLlmFails() {
        AgentTaskEntity task = task(3L, null);
        when(agentTaskStepRepository.findByTaskIdOrderByStepOrderAsc(3L)).thenReturn(steps(3L));
        when(ragTwoStageRetrievalService.retrieve(anyString(), eq(5), any(RetrievalScope.class)))
                .thenReturn(new RagRetrievalOutcome(
                        List.of(new RagRetrievedChunk(1, 1, 10L, "doc", 20L, 0.9, null, "content")),
                        5,
                        5,
                        false,
                        null,
                        0L
                ));
        when(llmProvider.generate(anyString(), anyString(), any(LlmGenerateOptions.class)))
                .thenReturn(LlmGenerateResult.success("摘要", MockLlmProvider.PROVIDER, MockLlmProvider.DEFAULT_MODEL, 5, 5, 1L, "stop", null))
                .thenReturn(LlmGenerateResult.failure(MockLlmProvider.PROVIDER, MockLlmProvider.DEFAULT_MODEL, "llm failed", 1L));

        assertThatThrownBy(() -> workflowService.executeWorkflow(task))
                .isInstanceOf(BusinessException.class)
                .extracting("errorCode")
                .isEqualTo(ErrorCode.AGENT_FINAL_REPORT_FAILED);

        assertThat(task.getStatus()).isEqualTo(AgentTaskStatus.FAILED);
        verify(agentTaskEventRecorder).recordFinalReportFailed(eq(3L), anyString(), anyString());
    }

    private AgentTaskEntity task(Long id, Long collectionId) {
        AgentTaskEntity task = new AgentTaskEntity();
        task.setId(id);
        task.setTitle("任务");
        task.setObjective("分析目标");
        task.setCollectionId(collectionId);
        task.setCollectionName(collectionId == null ? null : "分组");
        task.setStatus(AgentTaskStatus.RUNNING);
        return task;
    }

    private List<AgentTaskStepEntity> steps(Long taskId) {
        return List.of(
                step(taskId, 1, AgentTaskStepType.TOOL_CALL, AgentToolNames.KNOWLEDGE_SEARCH, "检索知识库"),
                step(taskId, 2, AgentTaskStepType.TOOL_CALL, AgentToolNames.CONTEXT_SUMMARY, "总结检索结果"),
                step(taskId, 3, AgentTaskStepType.FINAL_REPORT, AgentToolNames.FINAL_REPORT, "生成最终报告")
        );
    }

    private AgentTaskStepEntity step(Long taskId, int order, AgentTaskStepType type, String toolName, String title) {
        AgentTaskStepEntity step = new AgentTaskStepEntity();
        step.setId((long) order);
        step.setTaskId(taskId);
        step.setStepOrder(order);
        step.setStepType(type);
        step.setToolName(toolName);
        step.setTitle(title);
        step.setDisplayTitle(title);
        step.setStatus(AgentTaskStepStatus.PENDING);
        return step;
    }
}
