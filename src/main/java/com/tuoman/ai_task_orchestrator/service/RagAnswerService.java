package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.DocumentSearchRequest;
import com.tuoman.ai_task_orchestrator.dto.DocumentSearchResultResponse;
import com.tuoman.ai_task_orchestrator.dto.RagAnswerRequest;
import com.tuoman.ai_task_orchestrator.dto.RagAnswerResponse;
import com.tuoman.ai_task_orchestrator.dto.RagCitationResponse;
import com.tuoman.ai_task_orchestrator.dto.RagLlmMetadataResponse;
import com.tuoman.ai_task_orchestrator.dto.RagRetrievalMetadataResponse;
import com.tuoman.ai_task_orchestrator.embedding.MockEmbeddingClient;
import com.tuoman.ai_task_orchestrator.llm.LlmClient;
import com.tuoman.ai_task_orchestrator.llm.LlmRequest;
import com.tuoman.ai_task_orchestrator.llm.LlmResponse;
import com.tuoman.ai_task_orchestrator.llm.ModelRouter;
import lombok.RequiredArgsConstructor;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Service;
import org.springframework.web.server.ResponseStatusException;

import java.util.List;
import java.util.stream.IntStream;

@Service
@RequiredArgsConstructor
public class RagAnswerService {

    private static final int DEFAULT_TOP_K = 5;

    private static final int MAX_TOP_K = 20;

    private static final int CONTENT_PREVIEW_MAX_LENGTH = 160;

    private static final String UNKNOWN_ANSWER = "无法从当前资料中确定。";

    private final DocumentEmbeddingService documentEmbeddingService;

    private final RagPromptBuilder ragPromptBuilder;

    private final LlmClient llmClient;

    private final ModelRouter modelRouter;

    public RagAnswerResponse answer(RagAnswerRequest request) {
        if (request == null || request.getQuery() == null || request.getQuery().isBlank()) {
            throw new ResponseStatusException(HttpStatus.BAD_REQUEST, "query must not be blank");
        }

        int topK = normalizeTopK(request.getTopK());
        List<DocumentSearchResultResponse> chunks = searchChunks(request, topK);
        List<RagCitationResponse> citations = toCitations(chunks);
        RagRetrievalMetadataResponse retrieval = toRetrievalMetadata(topK, chunks);

        if (chunks.isEmpty()) {
            return new RagAnswerResponse(
                    request.getQuery(),
                    UNKNOWN_ANSWER,
                    citations,
                    retrieval,
                    new RagLlmMetadataResponse(null, null, null, null, null, null)
            );
        }

        String selectedModel = modelRouter.route(request.getRequestedModel());
        String prompt = ragPromptBuilder.buildPrompt(request.getQuery(), chunks);

        LlmRequest llmRequest = new LlmRequest();
        llmRequest.setPrompt(prompt);
        llmRequest.setModel(selectedModel);

        LlmResponse llmResponse = llmClient.generate(llmRequest);
        String answer = llmResponse != null && llmResponse.isSuccess()
                ? llmResponse.getContent()
                : UNKNOWN_ANSWER;

        return new RagAnswerResponse(
                request.getQuery(),
                answer,
                citations,
                retrieval,
                toLlmMetadata(llmResponse)
        );
    }

    private List<DocumentSearchResultResponse> searchChunks(RagAnswerRequest request, int topK) {
        DocumentSearchRequest searchRequest = new DocumentSearchRequest();
        searchRequest.setQuery(request.getQuery());
        searchRequest.setDocumentId(request.getDocumentId());
        searchRequest.setTopK(topK);
        return documentEmbeddingService.search(searchRequest);
    }

    private List<RagCitationResponse> toCitations(List<DocumentSearchResultResponse> chunks) {
        return IntStream.range(0, chunks.size())
                .mapToObj(index -> toCitation(index + 1, chunks.get(index)))
                .toList();
    }

    private RagCitationResponse toCitation(int citationId, DocumentSearchResultResponse chunk) {
        return new RagCitationResponse(
                citationId,
                chunk.getDocumentId(),
                chunk.getChunkId(),
                chunk.getChunkIndex(),
                chunk.getScore(),
                chunk.getHeadingPath(),
                chunk.getStartOffset(),
                chunk.getEndOffset(),
                chunk.getChunkStrategy(),
                contentPreview(chunk.getContent())
        );
    }

    private RagRetrievalMetadataResponse toRetrievalMetadata(int topK, List<DocumentSearchResultResponse> chunks) {
        if (chunks.isEmpty()) {
            return new RagRetrievalMetadataResponse(
                    topK,
                    0,
                    MockEmbeddingClient.PROVIDER,
                    MockEmbeddingClient.DEFAULT_MODEL,
                    MockEmbeddingClient.DISTANCE_METRIC
            );
        }

        DocumentSearchResultResponse first = chunks.getFirst();
        return new RagRetrievalMetadataResponse(
                topK,
                chunks.size(),
                first.getEmbeddingProvider(),
                first.getEmbeddingModel(),
                first.getDistanceMetric()
        );
    }

    private RagLlmMetadataResponse toLlmMetadata(LlmResponse response) {
        if (response == null) {
            return new RagLlmMetadataResponse(null, null, null, null, null, null);
        }

        return new RagLlmMetadataResponse(
                response.getProvider(),
                response.getModel(),
                response.getPromptTokenCount(),
                response.getCompletionTokenCount(),
                response.getTotalTokenCount(),
                response.getLatencyMs()
        );
    }

    private String contentPreview(String content) {
        if (content == null) {
            return null;
        }

        if (content.length() <= CONTENT_PREVIEW_MAX_LENGTH) {
            return content;
        }

        return content.substring(0, CONTENT_PREVIEW_MAX_LENGTH);
    }

    private int normalizeTopK(Integer topK) {
        if (topK == null || topK <= 0) {
            return DEFAULT_TOP_K;
        }

        return Math.min(topK, MAX_TOP_K);
    }
}
