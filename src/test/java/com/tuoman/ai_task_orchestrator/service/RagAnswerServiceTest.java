package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.dto.RagAnswerRequest;
import com.tuoman.ai_task_orchestrator.dto.RagAnswerResponse;
import com.tuoman.ai_task_orchestrator.dto.RagQualityDiagnosisResponse;
import com.tuoman.ai_task_orchestrator.dto.RagQualityScoreResponse;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingProvider;
import com.tuoman.ai_task_orchestrator.embedding.MockEmbeddingClient;
import com.tuoman.ai_task_orchestrator.llm.LlmClient;
import com.tuoman.ai_task_orchestrator.llm.LlmRequest;
import com.tuoman.ai_task_orchestrator.llm.LlmResponse;
import com.tuoman.ai_task_orchestrator.rag.quality.RagQualityMode;
import com.tuoman.ai_task_orchestrator.rag.quality.RagQualityService;
import com.tuoman.ai_task_orchestrator.config.RetrievalPipelineProperties;
import com.tuoman.ai_task_orchestrator.rerank.RagTwoStageRetrievalService.RagRetrievalOutcome;
import com.tuoman.ai_task_orchestrator.rerank.RagTwoStageRetrievalService.RagRetrievedChunk;
import com.tuoman.ai_task_orchestrator.vectorstore.ExactCosineVectorStore;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStore;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreProperties;
import com.tuoman.ai_task_orchestrator.retrieval.CollectionAskEmptyReason;
import com.tuoman.ai_task_orchestrator.retrieval.CollectionAskScope;
import com.tuoman.ai_task_orchestrator.retrieval.RetrievalScope;
import com.tuoman.ai_task_orchestrator.queryunderstanding.QueryClarificationGuard;
import com.tuoman.ai_task_orchestrator.queryunderstanding.QueryRewriteResult;
import com.tuoman.ai_task_orchestrator.queryunderstanding.QueryRewriteService;
import com.tuoman.ai_task_orchestrator.queryunderstanding.QueryType;
import com.tuoman.ai_task_orchestrator.queryunderstanding.QueryUnderstandingResult;
import com.tuoman.ai_task_orchestrator.queryunderstanding.QueryUnderstandingService;
import com.tuoman.ai_task_orchestrator.queryunderstanding.RetrievalRoutingDecision;
import com.tuoman.ai_task_orchestrator.queryunderstanding.RetrievalRoutingPolicyService;
import com.tuoman.ai_task_orchestrator.queryunderstanding.RetrievalRoutingStrategy;
import com.tuoman.ai_task_orchestrator.repository.DocumentCollectionRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.retrieval.RetrievalFilter;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.ArgumentCaptor;
import org.mockito.InjectMocks;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.List;
import java.util.Set;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyInt;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.lenient;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class RagAnswerServiceTest {

    @Mock
    private AppRetrievalService appRetrievalService;

    @Mock
    private RetrievalPipelineProperties retrievalPipelineProperties;

    @Mock
    private EmbeddingProvider embeddingProvider;

    @Mock
    private VectorStore vectorStore;

    @Mock
    private VectorStoreProperties vectorStoreProperties;

    @Mock
    private RagPromptBuilder ragPromptBuilder;

    @Mock
    private LlmClient llmClient;

    @Mock
    private CollectionScopeService collectionScopeService;

    @Mock
    private RagQualityService ragQualityService;

    @Mock
    private QueryUnderstandingService queryUnderstandingService;

    @Mock
    private QueryRewriteService queryRewriteService;

    @Mock
    private RetrievalRoutingPolicyService retrievalRoutingPolicyService;

    @Mock
    private QueryClarificationGuard queryClarificationGuard;

    @Mock
    private DocumentRepository documentRepository;

    @Mock
    private DocumentCollectionRepository documentCollectionRepository;

    @InjectMocks
    private RagAnswerService ragAnswerService;

    @BeforeEach
    void setUpProviderMetadata() {
        lenient().when(appRetrievalService.useV15Pipeline()).thenReturn(false);
        lenient().when(ragQualityService.evaluate(any(), any(), any(), any(), any(), any()))
                .thenReturn(new RagQualityScoreResponse(
                        80,
                        com.tuoman.ai_task_orchestrator.rag.quality.RagQualityLevel.GOOD,
                        "良好",
                        78,
                        80,
                        85,
                        86,
                        RagQualityMode.BALANCED,
                        "平衡模式",
                        java.util.Map.of("retrieval", 0.30, "context", 0.25, "answer", 0.25, "citation", 0.20),
                        new RagQualityDiagnosisResponse("测试摘要", List.of(), List.of()),
                        java.util.Map.of(),
                        "测试"
                ));
        lenient().when(embeddingProvider.provider()).thenReturn(MockEmbeddingClient.PROVIDER);
        lenient().when(embeddingProvider.runtimeProvider()).thenReturn(MockEmbeddingClient.PROVIDER);
        lenient().when(embeddingProvider.model()).thenReturn(MockEmbeddingClient.DEFAULT_MODEL);
        lenient().when(embeddingProvider.dimension()).thenReturn(MockEmbeddingClient.DIMENSION);
        lenient().when(vectorStoreProperties.getProvider()).thenReturn(ExactCosineVectorStore.PROVIDER);
        RetrievalRoutingDecision decision = RetrievalRoutingDecision.builder()
                .strategy(RetrievalRoutingStrategy.VECTOR_WITH_METADATA_FILTER)
                .filter(RetrievalFilter.empty())
                .vectorTopK(20)
                .keywordTopK(20)
                .finalTopK(5)
                .rerankTopN(5)
                .routingReasons(List.of())
                .warnings(List.of())
                .build();
        lenient().when(queryUnderstandingService.understand(anyString(), any(), any())).thenAnswer(invocation -> {
            String query = invocation.getArgument(0);
            return QueryUnderstandingResult.builder()
                    .originalQuery(query)
                    .normalizedQuery(query)
                    .queryType(QueryType.SINGLE_DOC_FACT)
                    .confidence(0.8)
                    .entities(List.of(query))
                    .codeSymbols(List.of())
                    .configKeys(List.of())
                    .apiPaths(List.of())
                    .tags(List.of())
                    .warnings(List.of())
                    .reasons(List.of())
                    .build();
        });
        lenient().when(queryRewriteService.rewrite(any())).thenAnswer(invocation -> {
            QueryUnderstandingResult result = invocation.getArgument(0);
            return new QueryRewriteResult(result.getOriginalQuery(), result.getOriginalQuery(), result.getOriginalQuery(), result.getOriginalQuery(), null, result.getOriginalQuery());
        });
        lenient().when(retrievalRoutingPolicyService.route(any(), any())).thenReturn(decision);
        lenient().when(queryClarificationGuard.evaluate(any(), any(), any(), any(Long.class), any(Long.class)))
                .thenReturn(new QueryClarificationGuard.GuardResult(false, null, List.of()));
        lenient().when(documentRepository.count()).thenReturn(1L);
        lenient().when(documentCollectionRepository.count()).thenReturn(1L);
    }

    @Test
    void answerShouldUseRetrievalOutcomeBuildPromptCallLlmAndReturnMetadata() {
        RagAnswerRequest request = request("Why use outbox?", 3);
        RagRetrievalOutcome outcome = new RagRetrievalOutcome(
                List.of(
                        chunk(1, 1, 10L, 0.9, null, "Outbox avoids dual write loss."),
                        chunk(2, 2, 11L, 0.8, null, "Atomic claim prevents duplicate execution.")
                ),
                3,
                3,
                false,
                null,
                0L
        );
        when(appRetrievalService.retrieve(eq("Why use outbox?"), eq(3), any(RetrievalScope.class), any(), any())).thenReturn(unified(outcome));
        when(ragPromptBuilder.buildPrompt(any(), any())).thenReturn("rag prompt");
        when(llmClient.generate(any(LlmRequest.class))).thenReturn(successResponse("Answer with [1] and [2]."));

        RagAnswerResponse response = ragAnswerService.answer(request);

        assertThat(response.getAnswer()).isEqualTo("Answer with [1] and [2].");
        assertThat(response.getCitations()).hasSize(2);
        assertThat(response.getCitations().get(0).getSourceIndex()).isEqualTo(1);
        assertThat(response.getCitations().get(0).getChunkId()).isEqualTo(10L);
        assertThat(response.getRetrieval().getTopK()).isEqualTo(3);
        assertThat(response.getRetrieval().getReturned()).isEqualTo(2);
        assertThat(response.getRetrieval().getRerankEnabled()).isFalse();
        assertThat(response.getRetrieval().getRerankerName()).isNull();
        verify(ragPromptBuilder).buildPrompt(request.getQuery(), response.getCitations());
        verify(llmClient).generate(any(LlmRequest.class));
    }

    @Test
    void answerShouldNotForceMockLlmModelOnRequest() {
        RagRetrievalOutcome outcome = new RagRetrievalOutcome(
                List.of(chunk(1, 1, 10L, 0.9, null, "context")),
                1,
                1,
                false,
                null,
                0L
        );
        when(appRetrievalService.retrieve(anyString(), anyInt(), any(RetrievalScope.class), any(), any())).thenReturn(unified(outcome));
        when(ragPromptBuilder.buildPrompt(any(), any())).thenReturn("rag prompt");
        when(llmClient.generate(any(LlmRequest.class))).thenReturn(successResponse("answer"));

        ragAnswerService.answer(request("question", 1));

        ArgumentCaptor<LlmRequest> captor = ArgumentCaptor.forClass(LlmRequest.class);
        verify(llmClient).generate(captor.capture());
        assertThat(captor.getValue().getModel()).isNull();
    }

    @Test
    void answerShouldExposeLlmMetadataFields() {
        RagRetrievalOutcome outcome = new RagRetrievalOutcome(
                List.of(chunk(1, 1, 10L, 0.9, null, "context")),
                1,
                1,
                false,
                null,
                0L
        );
        when(appRetrievalService.retrieve(anyString(), anyInt(), any(RetrievalScope.class), any(), any())).thenReturn(unified(outcome));
        when(ragPromptBuilder.buildPrompt(any(), any())).thenReturn("rag prompt");
        LlmResponse llmResponse = successResponse("answer with [1]");
        llmResponse.setProvider("local-ollama");
        llmResponse.setModel("qwen2.5:7b");
        when(llmClient.generate(any(LlmRequest.class))).thenReturn(llmResponse);

        RagAnswerResponse response = ragAnswerService.answer(request("question", 1));

        assertThat(response.getGeneration().getLlmProvider()).isEqualTo("local-ollama");
        assertThat(response.getGeneration().getLlmModel()).isEqualTo("qwen2.5:7b");
        assertThat(response.getGeneration().getProvider()).isEqualTo("local-ollama");
        assertThat(response.getGeneration().getModel()).isEqualTo("qwen2.5:7b");
    }

    @Test
    void answerShouldUseRerankedOrderWhenRerankEnabled() {
        RagRetrievalOutcome outcome = new RagRetrievalOutcome(
                List.of(chunk(1, 2, 20L, 0.5, 0.95, "cache key four tuple")),
                1,
                20,
                true,
                "lexical",
                3L
        );
        when(appRetrievalService.retrieve(eq("cache key"), eq(1), any(RetrievalScope.class), any(), any())).thenReturn(unified(outcome));
        when(ragPromptBuilder.buildPrompt(any(), any())).thenReturn("prompt");
        when(llmClient.generate(any())).thenReturn(successResponse("answer"));

        RagAnswerResponse response = ragAnswerService.answer(request("cache key", 1));

        assertThat(response.getCitations().getFirst().getChunkId()).isEqualTo(20L);
        assertThat(response.getCitations().getFirst().getOriginalRank()).isEqualTo(2);
        assertThat(response.getCitations().getFirst().getRerankedRank()).isEqualTo(1);
        assertThat(response.getCitations().getFirst().getRerankScore()).isEqualTo(0.95);
        assertThat(response.getRetrieval().getRerankEnabled()).isTrue();
        assertThat(response.getRetrieval().getRerankerName()).isEqualTo("lexical");
        assertThat(response.getRetrieval().getCandidateTopK()).isEqualTo(20);
        assertThat(response.getRetrieval().getRerankLatencyMs()).isEqualTo(3L);
    }

    @Test
    void answerShouldUseDefaultTopKWhenMissing() {
        when(appRetrievalService.retrieve(eq("query"), eq(5), any(RetrievalScope.class), any(), any()))
                .thenReturn(unified(new RagRetrievalOutcome(List.of(), 5, 5, false, null, 0L)));

        RagAnswerResponse response = ragAnswerService.answer(request("query", null));

        assertThat(response.getRetrieval().getTopK()).isEqualTo(5);
        verify(llmClient, never()).generate(any());
    }

    @Test
    void answerShouldRejectTopKBelowOne() {
        assertThatThrownBy(() -> ragAnswerService.answer(request("query", 0)))
                .isInstanceOf(BusinessException.class)
                .extracting("errorCode")
                .isEqualTo(com.tuoman.ai_task_orchestrator.common.error.ErrorCode.VALIDATION_ERROR);
    }

    @Test
    void answerShouldRejectTopKAboveMax() {
        assertThatThrownBy(() -> ragAnswerService.answer(request("query", 11)))
                .isInstanceOf(BusinessException.class)
                .extracting("errorCode")
                .isEqualTo(com.tuoman.ai_task_orchestrator.common.error.ErrorCode.VALIDATION_ERROR);
    }

    @Test
    void answerShouldReturnNoContextResponseWithoutCallingLlm() {
        when(appRetrievalService.retrieve(anyString(), anyInt(), any(RetrievalScope.class), any(), any()))
                .thenReturn(unified(new RagRetrievalOutcome(List.of(), 5, 5, false, null, 0L)));

        RagAnswerResponse response = ragAnswerService.answer(request("No context question", 5));

        assertThat(response.getAnswer()).isEqualTo("根据当前检索到的文档内容，无法确定。");
        assertThat(response.getCitations()).isEmpty();
        assertThat(response.getGeneration().getSkipped()).isTrue();
        verify(llmClient, never()).generate(any());
    }

    @Test
    void answerShouldLimitCitationSnippetLength() {
        String longContent = "a".repeat(500);
        when(appRetrievalService.retrieve(anyString(), anyInt(), any(RetrievalScope.class), any(), any()))
                .thenReturn(unified(new RagRetrievalOutcome(
                List.of(chunk(1, 1, 10L, 0.9, null, longContent)),
                1,
                1,
                false,
                null,
                0L
        )));
        when(ragPromptBuilder.buildPrompt(any(), any())).thenReturn("prompt");
        when(llmClient.generate(any())).thenReturn(successResponse("answer"));

        RagAnswerResponse response = ragAnswerService.answer(request("query", 1));

        assertThat(response.getCitations().getFirst().getContentSnippet()).hasSize(400);
    }

    @Test
    void answerShouldConvertLlmFailureToBusinessException() {
        when(appRetrievalService.retrieve(anyString(), anyInt(), any(RetrievalScope.class), any(), any()))
                .thenReturn(unified(new RagRetrievalOutcome(
                List.of(chunk(1, 1, 10L, 0.9, null, "content")),
                1,
                1,
                false,
                null,
                0L
        )));
        when(ragPromptBuilder.buildPrompt(any(), any())).thenReturn("prompt");

        LlmResponse failed = new LlmResponse();
        failed.setSuccess(false);
        failed.setErrorMessage("llm failed");
        when(llmClient.generate(any())).thenReturn(failed);

        assertThatThrownBy(() -> ragAnswerService.answer(request("query", 1)))
                .isInstanceOf(BusinessException.class)
                .extracting("errorCode")
                .isEqualTo(com.tuoman.ai_task_orchestrator.common.error.ErrorCode.LLM_PROVIDER_ERROR);
    }

    @Test
    void answerShouldReturnCollectionEmptyMessageWithoutCallingLlm() {
        when(collectionScopeService.resolveForAsk(9L)).thenReturn(new CollectionAskScope(
                9L,
                "项目 A",
                CollectionAskEmptyReason.NO_DOCUMENTS,
                Set.of(),
                Set.of(),
                "当前分组下没有可用于问答的文档，请先添加已启用文档。"
        ));

        RagAnswerRequest request = request("scoped question", 5);
        request.setCollectionId(9L);

        RagAnswerResponse response = ragAnswerService.answer(request);

        assertThat(response.getAnswer()).contains("当前分组下没有可用于问答的文档");
        assertThat(response.getCitations()).isEmpty();
        assertThat(response.getRetrieval().getScopeType()).isEqualTo("COLLECTION");
        assertThat(response.getRetrieval().getCollectionId()).isEqualTo(9L);
        assertThat(response.getGeneration().getSkipped()).isTrue();
        verify(appRetrievalService, never()).retrieve(anyString(), anyInt(), any(RetrievalScope.class), any(), any());
        verify(llmClient, never()).generate(any());
    }

    @Test
    void answerShouldPassCollectionScopeToRetrieval() {
        when(collectionScopeService.resolveForAsk(2L)).thenReturn(new CollectionAskScope(
                2L,
                "项目 B",
                CollectionAskEmptyReason.NONE,
                Set.of(10L),
                Set.of(10L),
                null
        ));
        RagRetrievalOutcome outcome = new RagRetrievalOutcome(
                List.of(chunk(1, 1, 30L, 0.9, null, "scoped content")),
                5,
                5,
                false,
                null,
                0L
        );
        when(appRetrievalService.retrieve(eq("scoped"), eq(5), any(RetrievalScope.class), any(), any())).thenReturn(unified(outcome));
        when(ragPromptBuilder.buildPrompt(any(), any())).thenReturn("prompt");
        when(llmClient.generate(any())).thenReturn(successResponse("scoped answer"));

        RagAnswerRequest request = request("scoped", 5);
        request.setCollectionId(2L);
        RagAnswerResponse response = ragAnswerService.answer(request);

        assertThat(response.getAnswer()).isEqualTo("scoped answer");
        assertThat(response.getRetrieval().getCollectionName()).isEqualTo("项目 B");
    }

    @Test
    void askShouldExposeQueryUnderstandingDiagnosticsAndPassRoutingDecision() {
        RetrievalRoutingDecision decision = RetrievalRoutingDecision.builder()
                .strategy(RetrievalRoutingStrategy.HYBRID_RRF_RERANK)
                .filter(RetrievalFilter.builder().version("V10.0").build())
                .vectorTopK(30)
                .keywordTopK(50)
                .finalTopK(8)
                .rerankTopN(8)
                .routingReasons(List.of("test"))
                .warnings(List.of())
                .build();
        when(retrievalRoutingPolicyService.route(any(), any())).thenReturn(decision);
        when(appRetrievalService.retrieve(anyString(), anyInt(), any(RetrievalScope.class), any(), eq(decision)))
                .thenReturn(unified(new RagRetrievalOutcome(
                        List.of(chunk(1, 1, 10L, 0.9, null, "content")),
                        8,
                        30,
                        true,
                        "lexical",
                        0L
                )));
        when(ragPromptBuilder.buildPrompt(any(), any())).thenReturn("prompt");
        when(llmClient.generate(any())).thenReturn(successResponse("answer"));

        RagAnswerResponse response = ragAnswerService.answer(request("V10.0 LocalPythonLlmProvider", 5));

        assertThat(response.getQueryUnderstanding()).isNotNull();
        assertThat(response.getQueryUnderstanding().getRoutingDecision().getStrategy()).isEqualTo(RetrievalRoutingStrategy.HYBRID_RRF_RERANK);
        verify(appRetrievalService).retrieve(anyString(), eq(5), any(RetrievalScope.class), any(), eq(decision));
    }

    @Test
    void ambiguousQueryShouldNotBlindSearch() {
        QueryUnderstandingResult ambiguous = QueryUnderstandingResult.builder()
                .originalQuery("啥")
                .normalizedQuery("啥")
                .queryType(QueryType.AMBIGUOUS)
                .confidence(0.2)
                .entities(List.of())
                .codeSymbols(List.of())
                .configKeys(List.of())
                .apiPaths(List.of())
                .warnings(List.of())
                .reasons(List.of())
                .build();
        when(queryUnderstandingService.understand(eq("啥"), any(), any())).thenReturn(ambiguous);
        when(queryClarificationGuard.evaluate(any(), any(), any(), any(Long.class), any(Long.class)))
                .thenReturn(new QueryClarificationGuard.GuardResult(
                        true,
                        "当前问题比较模糊，建议先选择知识库分组，避免跨项目文档混入。",
                        List.of("queryType=AMBIGUOUS")
                ));

        RagAnswerResponse response = ragAnswerService.answer(request("啥", 5));

        assertThat(response.getAnswer()).contains("当前问题比较模糊");
        assertThat(response.getQueryUnderstanding().isClarificationRequired()).isTrue();
        verify(appRetrievalService, never()).retrieve(anyString(), anyInt(), any(RetrievalScope.class), any(), any());
    }

    private RagAnswerRequest request(String query, Integer topK) {
        RagAnswerRequest request = new RagAnswerRequest();
        request.setQuery(query);
        request.setTopK(topK);
        return request;
    }

    private RagRetrievedChunk chunk(
            int rerankedRank,
            int originalRank,
            Long chunkId,
            Double originalScore,
            Double rerankScore,
            String content
    ) {
        return new RagRetrievedChunk(
                rerankedRank,
                originalRank,
                1L,
                "heading",
                chunkId,
                originalScore,
                rerankScore,
                content
        );
    }

    private AppRetrievalService.UnifiedRetrievalOutcome unified(RagRetrievalOutcome outcome) {
        return new AppRetrievalService.UnifiedRetrievalOutcome(outcome, null, false);
    }

    private LlmResponse successResponse(String content) {
        LlmResponse response = new LlmResponse();
        response.setProvider("mock");
        response.setModel("mock-llm");
        response.setContent(content);
        response.setSuccess(true);
        return response;
    }
}
