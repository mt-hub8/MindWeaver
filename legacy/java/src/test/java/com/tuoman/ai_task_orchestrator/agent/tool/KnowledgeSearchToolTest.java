package com.tuoman.ai_task_orchestrator.agent.tool;

import com.tuoman.ai_task_orchestrator.agent.AgentTaskProperties;
import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.common.error.ErrorCode;
import com.tuoman.ai_task_orchestrator.retrieval.CollectionAskEmptyReason;
import com.tuoman.ai_task_orchestrator.retrieval.CollectionAskScope;
import com.tuoman.ai_task_orchestrator.retrieval.RetrievalScope;
import com.tuoman.ai_task_orchestrator.rerank.RagTwoStageRetrievalService;
import com.tuoman.ai_task_orchestrator.rerank.RagTwoStageRetrievalService.RagRetrievalOutcome;
import com.tuoman.ai_task_orchestrator.rerank.RagTwoStageRetrievalService.RagRetrievedChunk;
import com.tuoman.ai_task_orchestrator.service.CollectionScopeService;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.ArgumentCaptor;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.List;
import java.util.Map;
import java.util.Set;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyInt;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class KnowledgeSearchToolTest {

    @Mock
    private CollectionScopeService collectionScopeService;

    @Mock
    private RagTwoStageRetrievalService ragTwoStageRetrievalService;

    private KnowledgeSearchTool knowledgeSearchTool;

    @BeforeEach
    void setUp() {
        AgentTaskProperties properties = new AgentTaskProperties();
        properties.setDefaultTopK(5);
        knowledgeSearchTool = new KnowledgeSearchTool(collectionScopeService, ragTwoStageRetrievalService, properties);
    }

    @Test
    void executeShouldReturnNoContextWhenCollectionEmpty() {
        when(collectionScopeService.resolveForAsk(9L)).thenReturn(new CollectionAskScope(
                9L,
                "空分组",
                CollectionAskEmptyReason.NO_DOCUMENTS,
                Set.of(),
                Set.of(),
                "当前分组下没有可用于问答的文档"
        ));

        ToolExecutionResult result = knowledgeSearchTool.execute(
                Map.of("query", "目标", "collectionId", 9L),
                new ToolExecutionContext(1L, "t", "目标", 9L, "空分组", "scope")
        );

        assertThat(result.noContext()).isTrue();
        assertThat(result.output().get("noContext")).isEqualTo(true);
    }

    @Test
    void executeShouldUseCollectionScopeWhenCollectionProvided() {
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

        knowledgeSearchTool.execute(
                Map.of("query", "目标", "collectionId", 2L),
                new ToolExecutionContext(1L, "t", "目标", 2L, "项目 B", "scope")
        );

        ArgumentCaptor<RetrievalScope> scopeCaptor = ArgumentCaptor.forClass(RetrievalScope.class);
        verify(ragTwoStageRetrievalService).retrieve(anyString(), eq(5), scopeCaptor.capture());
        assertThat(scopeCaptor.getValue().isCollectionScoped()).isTrue();
        assertThat(scopeCaptor.getValue().collectionId()).isEqualTo(2L);
    }

    @Test
    void executeShouldFailWhenQueryMissing() {
        assertThatThrownBy(() -> knowledgeSearchTool.execute(Map.of(), new ToolExecutionContext(
                1L, "t", "目标", null, null, "全部文档"
        ))).isInstanceOf(BusinessException.class)
                .extracting("errorCode")
                .isEqualTo(ErrorCode.AGENT_TOOL_INPUT_INVALID);
    }

    @Test
    void executeShouldReturnCitationsWhenChunksFound() {
        when(ragTwoStageRetrievalService.retrieve(anyString(), anyInt(), any(RetrievalScope.class)))
                .thenReturn(new RagRetrievalOutcome(
                        List.of(new RagRetrievedChunk(1, 1, 100L, "doc", 200L, 0.9, null, "content")),
                        5,
                        5,
                        false,
                        null,
                        0L
                ));

        ToolExecutionResult result = knowledgeSearchTool.execute(
                Map.of("query", "目标"),
                new ToolExecutionContext(1L, "t", "目标", null, null, "全部文档")
        );

        assertThat(result.success()).isTrue();
        assertThat(result.noContext()).isFalse();
        assertThat(result.output().get("citations")).isInstanceOf(List.class);
        assertThat(((List<?>) result.output().get("citations"))).hasSize(1);
    }
}
