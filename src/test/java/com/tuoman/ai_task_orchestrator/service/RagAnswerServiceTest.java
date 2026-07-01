package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.DocumentSearchRequest;
import com.tuoman.ai_task_orchestrator.dto.DocumentSearchResultResponse;
import com.tuoman.ai_task_orchestrator.dto.RagAnswerRequest;
import com.tuoman.ai_task_orchestrator.dto.RagAnswerResponse;
import com.tuoman.ai_task_orchestrator.llm.LlmClient;
import com.tuoman.ai_task_orchestrator.llm.LlmRequest;
import com.tuoman.ai_task_orchestrator.llm.LlmResponse;
import com.tuoman.ai_task_orchestrator.llm.ModelRouter;
import org.junit.jupiter.api.Test;
import org.mockito.ArgumentCaptor;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class RagAnswerServiceTest {

    private final DocumentEmbeddingService documentEmbeddingService = mock(DocumentEmbeddingService.class);

    private final RagPromptBuilder ragPromptBuilder = mock(RagPromptBuilder.class);

    private final LlmClient llmClient = mock(LlmClient.class);

    private final ModelRouter modelRouter = mock(ModelRouter.class);

    private final RagAnswerService ragAnswerService = new RagAnswerService(
            documentEmbeddingService,
            ragPromptBuilder,
            llmClient,
            modelRouter
    );

    @Test
    void answerShouldSearchBuildPromptCallLlmAndReturnMetadata() {
        RagAnswerRequest request = request("Why use outbox?", 1L, 3, "mock-smart");
        List<DocumentSearchResultResponse> chunks = List.of(
                chunk(1L, 10L, 0, 0.9, "Outbox", "Outbox avoids dual write loss."),
                chunk(1L, 11L, 1, 0.8, "Claim", "Atomic claim prevents duplicate execution.")
        );
        LlmResponse llmResponse = successResponse("Answer with [1] and [2].");

        when(documentEmbeddingService.search(any(DocumentSearchRequest.class))).thenReturn(chunks);
        when(modelRouter.route("mock-smart")).thenReturn("mock-smart");
        when(ragPromptBuilder.buildPrompt("Why use outbox?", chunks)).thenReturn("rag prompt");
        when(llmClient.generate(any(LlmRequest.class))).thenReturn(llmResponse);

        RagAnswerResponse response = ragAnswerService.answer(request);

        assertThat(response.getQuery()).isEqualTo("Why use outbox?");
        assertThat(response.getAnswer()).isEqualTo("Answer with [1] and [2].");
        assertThat(response.getCitations()).hasSize(2);
        assertThat(response.getCitations().get(0).getCitationId()).isEqualTo(1);
        assertThat(response.getCitations().get(0).getChunkId()).isEqualTo(10L);
        assertThat(response.getCitations().get(1).getCitationId()).isEqualTo(2);
        assertThat(response.getRetrieval().getTopK()).isEqualTo(3);
        assertThat(response.getRetrieval().getReturnedCount()).isEqualTo(2);
        assertThat(response.getRetrieval().getEmbeddingProvider()).isEqualTo("mock");
        assertThat(response.getLlm().getProvider()).isEqualTo("mock");
        assertThat(response.getLlm().getModel()).isEqualTo("mock-smart");
        assertThat(response.getLlm().getTotalTokenCount()).isEqualTo(9);

        ArgumentCaptor<DocumentSearchRequest> searchRequestCaptor = ArgumentCaptor.forClass(DocumentSearchRequest.class);
        verify(documentEmbeddingService).search(searchRequestCaptor.capture());
        assertThat(searchRequestCaptor.getValue().getQuery()).isEqualTo("Why use outbox?");
        assertThat(searchRequestCaptor.getValue().getDocumentId()).isEqualTo(1L);
        assertThat(searchRequestCaptor.getValue().getTopK()).isEqualTo(3);

        ArgumentCaptor<LlmRequest> llmRequestCaptor = ArgumentCaptor.forClass(LlmRequest.class);
        verify(llmClient).generate(llmRequestCaptor.capture());
        assertThat(llmRequestCaptor.getValue().getPrompt()).isEqualTo("rag prompt");
        assertThat(llmRequestCaptor.getValue().getModel()).isEqualTo("mock-smart");
    }

    @Test
    void answerShouldApplyDefaultAndMaxTopK() {
        when(documentEmbeddingService.search(any(DocumentSearchRequest.class))).thenReturn(List.of());

        ragAnswerService.answer(request("query", null, null, null));
        ragAnswerService.answer(request("query", null, 100, null));

        ArgumentCaptor<DocumentSearchRequest> captor = ArgumentCaptor.forClass(DocumentSearchRequest.class);
        verify(documentEmbeddingService, org.mockito.Mockito.times(2)).search(captor.capture());

        assertThat(captor.getAllValues().get(0).getTopK()).isEqualTo(5);
        assertThat(captor.getAllValues().get(1).getTopK()).isEqualTo(20);
    }

    @Test
    void answerShouldUseDefaultTopKWhenTopKIsNotPositive() {
        when(documentEmbeddingService.search(any(DocumentSearchRequest.class))).thenReturn(List.of());

        RagAnswerResponse response = ragAnswerService.answer(request("query", null, 0, null));

        assertThat(response.getRetrieval().getTopK()).isEqualTo(5);
        verify(llmClient, never()).generate(any());
    }

    @Test
    void answerShouldReturnUnknownAnswerWithoutCallingLlmWhenNoChunks() {
        when(documentEmbeddingService.search(any(DocumentSearchRequest.class))).thenReturn(List.of());

        RagAnswerResponse response = ragAnswerService.answer(request("No context question", 2L, 5, "mock-fast"));

        assertThat(response.getAnswer()).isEqualTo("无法从当前资料中确定。");
        assertThat(response.getCitations()).isEmpty();
        assertThat(response.getRetrieval().getReturnedCount()).isZero();
        assertThat(response.getRetrieval().getEmbeddingProvider()).isEqualTo("mock");
        assertThat(response.getRetrieval().getEmbeddingModel()).isEqualTo("mock-embedding-v1");
        assertThat(response.getRetrieval().getDistanceMetric()).isEqualTo("COSINE");
        assertThat(response.getLlm().getProvider()).isNull();
        verify(modelRouter, never()).route(any());
        verify(llmClient, never()).generate(any());
    }

    @Test
    void answerShouldLimitCitationPreviewLength() {
        String longContent = "a".repeat(200);
        RagAnswerRequest request = request("query", null, 1, "mock-llm");
        List<DocumentSearchResultResponse> chunks = List.of(chunk(1L, 10L, 0, 0.9, "Long", longContent));

        when(documentEmbeddingService.search(any(DocumentSearchRequest.class))).thenReturn(chunks);
        when(modelRouter.route("mock-llm")).thenReturn("mock-llm");
        when(ragPromptBuilder.buildPrompt("query", chunks)).thenReturn("prompt");
        when(llmClient.generate(any())).thenReturn(successResponse("answer"));

        RagAnswerResponse response = ragAnswerService.answer(request);

        assertThat(response.getCitations().getFirst().getContentPreview()).hasSize(160);
    }

    private RagAnswerRequest request(String query, Long documentId, Integer topK, String requestedModel) {
        RagAnswerRequest request = new RagAnswerRequest();
        request.setQuery(query);
        request.setDocumentId(documentId);
        request.setTopK(topK);
        request.setRequestedModel(requestedModel);
        return request;
    }

    private DocumentSearchResultResponse chunk(
            Long documentId,
            Long chunkId,
            Integer chunkIndex,
            Double score,
            String headingPath,
            String content
    ) {
        return new DocumentSearchResultResponse(
                documentId,
                chunkId,
                chunkIndex,
                score,
                content,
                content.length(),
                headingPath,
                10,
                10 + content.length(),
                "RECURSIVE_TEXT",
                "mock",
                "mock-embedding-v1",
                "COSINE"
        );
    }

    private LlmResponse successResponse(String content) {
        LlmResponse response = new LlmResponse();
        response.setProvider("mock");
        response.setModel("mock-smart");
        response.setContent(content);
        response.setSuccess(true);
        response.setPromptTokenCount(4);
        response.setCompletionTokenCount(5);
        response.setTotalTokenCount(9);
        response.setLatencyMs(12L);
        return response;
    }
}
