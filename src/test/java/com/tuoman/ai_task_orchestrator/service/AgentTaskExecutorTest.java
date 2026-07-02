package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.agent.AgentTaskEventRecorder;
import com.tuoman.ai_task_orchestrator.agent.AgentTaskPromptBuilder;
import com.tuoman.ai_task_orchestrator.agent.AgentTaskProperties;
import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.common.error.ErrorCode;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingProvider;
import com.tuoman.ai_task_orchestrator.embedding.MockEmbeddingClient;
import com.tuoman.ai_task_orchestrator.entity.AgentTaskEntity;
import com.tuoman.ai_task_orchestrator.enums.AgentTaskStatus;
import com.tuoman.ai_task_orchestrator.llm.LlmGenerateOptions;
import com.tuoman.ai_task_orchestrator.llm.LlmGenerateResult;
import com.tuoman.ai_task_orchestrator.llm.LlmProvider;
import com.tuoman.ai_task_orchestrator.llm.MockLlmProvider;
import com.tuoman.ai_task_orchestrator.repository.AgentTaskCitationRepository;
import com.tuoman.ai_task_orchestrator.repository.AgentTaskRepository;
import com.tuoman.ai_task_orchestrator.retrieval.CollectionAskEmptyReason;
import com.tuoman.ai_task_orchestrator.retrieval.CollectionAskScope;
import com.tuoman.ai_task_orchestrator.retrieval.RetrievalScope;
import com.tuoman.ai_task_orchestrator.rerank.RagTwoStageRetrievalService;
import com.tuoman.ai_task_orchestrator.rerank.RagTwoStageRetrievalService.RagRetrievalOutcome;
import com.tuoman.ai_task_orchestrator.rerank.RagTwoStageRetrievalService.RagRetrievedChunk;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.ArgumentCaptor;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.List;
import java.util.Optional;
import java.util.Set;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyInt;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class AgentTaskExecutorTest {

    @Mock
    private AgentTaskRepository agentTaskRepository;

    @Mock
    private AgentTaskCitationRepository agentTaskCitationRepository;

    @Mock
    private AgentTaskEventRecorder agentTaskEventRecorder;

    @Mock
    private CollectionScopeService collectionScopeService;

    @Mock
    private RagTwoStageRetrievalService ragTwoStageRetrievalService;

    @Mock
    private LlmProvider llmProvider;

    @Mock
    private EmbeddingProvider embeddingProvider;

    private AgentTaskExecutor agentTaskExecutor;

    @BeforeEach
    void setUp() {
        AgentTaskProperties properties = new AgentTaskProperties();
        properties.setDefaultTopK(5);
        agentTaskExecutor = new AgentTaskExecutor(
                agentTaskRepository,
                agentTaskCitationRepository,
                agentTaskEventRecorder,
                collectionScopeService,
                ragTwoStageRetrievalService,
                new AgentTaskPromptBuilder(),
                llmProvider,
                embeddingProvider,
                properties
        );
        when(embeddingProvider.provider()).thenReturn(MockEmbeddingClient.PROVIDER);
        when(embeddingProvider.model()).thenReturn(MockEmbeddingClient.DEFAULT_MODEL);
    }

    @Test
    void executeShouldCompleteWithLlmResult() {
        AgentTaskEntity task = pendingTask(1L, null);
        when(agentTaskRepository.findById(1L)).thenReturn(Optional.of(task));
        when(agentTaskRepository.save(any(AgentTaskEntity.class))).thenAnswer(invocation -> invocation.getArgument(0));

        RagRetrievalOutcome outcome = new RagRetrievalOutcome(
                List.of(chunk(1, 10L, "active content")),
                5,
                5,
                false,
                null,
                0L
        );
        when(ragTwoStageRetrievalService.retrieve(anyString(), eq(5), any(RetrievalScope.class))).thenReturn(outcome);
        when(llmProvider.generate(anyString(), anyString(), any(LlmGenerateOptions.class))).thenReturn(
                LlmGenerateResult.success(
                        "## 任务结论\n完成",
                        MockLlmProvider.PROVIDER,
                        MockLlmProvider.DEFAULT_MODEL,
                        100,
                        50,
                        5L,
                        "stop",
                        null
                )
        );

        agentTaskExecutor.execute(1L);

        assertThat(task.getStatus()).isEqualTo(AgentTaskStatus.COMPLETED);
        assertThat(task.getResult()).contains("任务结论");
        assertThat(task.getCitationCount()).isEqualTo(1);
        verify(agentTaskEventRecorder).recordTaskStarted(1L);
        verify(agentTaskEventRecorder).recordRetrievalStarted(1L);
        verify(agentTaskEventRecorder).recordLlmStarted(1L);
        verify(agentTaskEventRecorder).recordTaskCompleted(1L);
    }

    @Test
    void executeShouldCompleteWithoutLlmWhenNoContext() {
        AgentTaskEntity task = pendingTask(2L, 9L);
        when(agentTaskRepository.findById(2L)).thenReturn(Optional.of(task));
        when(agentTaskRepository.save(any(AgentTaskEntity.class))).thenAnswer(invocation -> invocation.getArgument(0));
        when(collectionScopeService.resolveForAsk(9L)).thenReturn(new CollectionAskScope(
                9L,
                "空分组",
                CollectionAskEmptyReason.NO_DOCUMENTS,
                Set.of(),
                Set.of(),
                "当前分组下没有可用于问答的文档，请先添加已启用文档。"
        ));

        agentTaskExecutor.execute(2L);

        assertThat(task.getStatus()).isEqualTo(AgentTaskStatus.COMPLETED);
        assertThat(task.getResult()).contains("当前分组下没有可用于问答的文档");
        verify(llmProvider, never()).generate(anyString(), anyString(), any());
        verify(ragTwoStageRetrievalService, never()).retrieve(anyString(), anyInt(), any());
    }

    @Test
    void executeShouldUseCollectionScopeWhenCollectionProvided() {
        AgentTaskEntity task = pendingTask(3L, 2L);
        task.setCollectionName("项目 B");
        when(agentTaskRepository.findById(3L)).thenReturn(Optional.of(task));
        when(agentTaskRepository.save(any(AgentTaskEntity.class))).thenAnswer(invocation -> invocation.getArgument(0));
        when(collectionScopeService.resolveForAsk(2L)).thenReturn(new CollectionAskScope(
                2L,
                "项目 B",
                CollectionAskEmptyReason.NONE,
                Set.of(10L),
                Set.of(10L),
                null
        ));
        when(ragTwoStageRetrievalService.retrieve(anyString(), eq(5), any(RetrievalScope.class)))
                .thenReturn(new RagRetrievalOutcome(List.of(), 5, 5, false, null, 0L));

        agentTaskExecutor.execute(3L);

        ArgumentCaptor<RetrievalScope> scopeCaptor = ArgumentCaptor.forClass(RetrievalScope.class);
        verify(ragTwoStageRetrievalService).retrieve(anyString(), eq(5), scopeCaptor.capture());
        assertThat(scopeCaptor.getValue().isCollectionScoped()).isTrue();
        assertThat(scopeCaptor.getValue().collectionId()).isEqualTo(2L);
    }

    @Test
    void executeShouldMarkFailedWhenLlmFails() {
        AgentTaskEntity task = pendingTask(4L, null);
        when(agentTaskRepository.findById(4L)).thenReturn(Optional.of(task));
        when(agentTaskRepository.save(any(AgentTaskEntity.class))).thenAnswer(invocation -> invocation.getArgument(0));
        when(ragTwoStageRetrievalService.retrieve(anyString(), eq(5), any(RetrievalScope.class)))
                .thenReturn(new RagRetrievalOutcome(List.of(chunk(1, 11L, "content")), 5, 5, false, null, 0L));
        when(llmProvider.generate(anyString(), anyString(), any(LlmGenerateOptions.class)))
                .thenReturn(LlmGenerateResult.failure(MockLlmProvider.PROVIDER, MockLlmProvider.DEFAULT_MODEL, "llm failed", 1L));

        assertThatThrownBy(() -> agentTaskExecutor.execute(4L))
                .isInstanceOf(BusinessException.class)
                .extracting("errorCode")
                .isEqualTo(ErrorCode.AGENT_TASK_LLM_FAILED);

        assertThat(task.getStatus()).isEqualTo(AgentTaskStatus.FAILED);
        verify(agentTaskEventRecorder).recordTaskFailed(eq(4L), any(), any(), any(), any(), any());
    }

    private AgentTaskEntity pendingTask(Long id, Long collectionId) {
        AgentTaskEntity task = new AgentTaskEntity();
        task.setId(id);
        task.setTitle("任务");
        task.setObjective("分析目标");
        task.setCollectionId(collectionId);
        task.setStatus(AgentTaskStatus.PENDING);
        return task;
    }

    private RagRetrievedChunk chunk(int rank, Long chunkId, String content) {
        return new RagRetrievedChunk(rank, rank, 1L, "doc", chunkId, 0.9, null, content);
    }
}
