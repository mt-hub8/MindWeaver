package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.dto.RagAnswerRequest;
import com.tuoman.ai_task_orchestrator.dto.RagAnswerResponse;
import com.tuoman.ai_task_orchestrator.dto.RagCitationResponse;
import com.tuoman.ai_task_orchestrator.dto.RagGenerationMetadataResponse;
import com.tuoman.ai_task_orchestrator.dto.RagRetrievalMetadataResponse;
import com.tuoman.ai_task_orchestrator.llm.LlmClient;
import com.tuoman.ai_task_orchestrator.llm.LlmRequest;
import com.tuoman.ai_task_orchestrator.llm.LlmResponse;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingProvider;
import com.tuoman.ai_task_orchestrator.rerank.RagTwoStageRetrievalService;
import com.tuoman.ai_task_orchestrator.rerank.RagTwoStageRetrievalService.RagRetrievalOutcome;
import com.tuoman.ai_task_orchestrator.rerank.RagTwoStageRetrievalService.RagRetrievedChunk;
import com.tuoman.ai_task_orchestrator.vectorstore.ExactCosineVectorStore;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStore;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreProperties;
import com.tuoman.ai_task_orchestrator.vectorstore.qdrant.QdrantVectorStore;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;

import java.util.List;

@Service
@RequiredArgsConstructor
public class RagAnswerService {

    private static final int DEFAULT_TOP_K = 5;

    private static final int MAX_TOP_K = 10;

    private static final int CONTENT_SNIPPET_MAX_LENGTH = 400;

    private static final String NO_CONTEXT_ANSWER = "根据当前检索到的文档内容，无法确定。";

    private static final String NO_CONTEXT_REASON = "NO_RETRIEVED_CONTEXT";

    private static final String DEFAULT_LLM_MODEL = "mock-llm";

    private final RagTwoStageRetrievalService ragTwoStageRetrievalService;

    private final EmbeddingProvider embeddingProvider;

    private final VectorStore vectorStore;

    private final VectorStoreProperties vectorStoreProperties;

    private final RagPromptBuilder ragPromptBuilder;

    private final LlmClient llmClient;

    public RagAnswerResponse answer(RagAnswerRequest request) {
        if (request == null) {
            throw BusinessException.validationError("request must not be null");
        }

        int topK = normalizeTopK(request.getTopK());
        RagRetrievalOutcome retrievalOutcome = ragTwoStageRetrievalService.retrieve(request.getQuery(), topK);
        List<RagCitationResponse> citations = toCitations(
                retrievalOutcome,
                retrievalOutcome.rerankEnabled(),
                retrievalOutcome.hybridEnabled()
        );
        RagRetrievalMetadataResponse retrieval = toRetrievalMetadata(retrievalOutcome);

        if (citations.isEmpty()) {
            return new RagAnswerResponse(
                    NO_CONTEXT_ANSWER,
                    List.of(),
                    retrieval,
                    new RagGenerationMetadataResponse(null, null, true, NO_CONTEXT_REASON)
            );
        }

        String prompt = ragPromptBuilder.buildPrompt(request.getQuery(), citations);

        LlmRequest llmRequest = new LlmRequest();
        llmRequest.setPrompt(prompt);
        llmRequest.setModel(DEFAULT_LLM_MODEL);

        LlmResponse llmResponse = llmClient.generate(llmRequest);
        if (llmResponse == null || !llmResponse.isSuccess() || llmResponse.getContent() == null || llmResponse.getContent().isBlank()) {
            String message = llmResponse == null || llmResponse.getErrorMessage() == null
                    ? "LLM provider error"
                    : llmResponse.getErrorMessage();
            throw BusinessException.llmProviderError(message);
        }

        return new RagAnswerResponse(
                llmResponse.getContent(),
                citations,
                retrieval,
                new RagGenerationMetadataResponse(
                        llmResponse.getProvider(),
                        llmResponse.getModel(),
                        null,
                        null
                )
        );
    }

    private List<RagCitationResponse> toCitations(
            RagRetrievalOutcome outcome,
            boolean rerankEnabled,
            boolean hybridEnabled
    ) {
        return outcome.chunks().stream()
                .map(chunk -> toCitation(chunk, rerankEnabled, hybridEnabled))
                .toList();
    }

    private RagCitationResponse toCitation(RagRetrievedChunk chunk, boolean rerankEnabled, boolean hybridEnabled) {
        Double displayScore;
        if (rerankEnabled) {
            displayScore = chunk.rerankScore();
        } else if (hybridEnabled) {
            displayScore = chunk.fusionScore() != null ? chunk.fusionScore() : chunk.originalScore();
        } else {
            displayScore = chunk.originalScore();
        }
        return new RagCitationResponse(
                chunk.rerankedRank(),
                chunk.documentId(),
                chunk.chunkId(),
                displayScore,
                contentSnippet(chunk.content()),
                rerankEnabled ? chunk.originalRank() : null,
                rerankEnabled ? chunk.rerankedRank() : null,
                rerankEnabled ? chunk.originalScore() : null,
                rerankEnabled ? chunk.rerankScore() : null,
                hybridEnabled ? chunk.denseRank() : null,
                hybridEnabled ? chunk.lexicalRank() : null,
                hybridEnabled ? chunk.denseScore() : null,
                hybridEnabled ? chunk.lexicalScore() : null,
                hybridEnabled ? chunk.fusionScore() : null,
                hybridEnabled ? chunk.denseHit() : null,
                hybridEnabled ? chunk.lexicalHit() : null
        );
    }

    private RagRetrievalMetadataResponse toRetrievalMetadata(RagRetrievalOutcome outcome) {
        return new RagRetrievalMetadataResponse(
                outcome.finalTopK(),
                outcome.chunks().size(),
                embeddingProvider.provider(),
                embeddingProvider.model(),
                embeddingProvider.dimension(),
                resolveVectorStoreName(),
                outcome.rerankEnabled(),
                outcome.rerankerName(),
                outcome.rerankEnabled() || outcome.hybridEnabled() ? outcome.candidateTopK() : null,
                outcome.finalTopK(),
                outcome.rerankEnabled() ? outcome.rerankLatencyMs() : null,
                outcome.hybridEnabled(),
                outcome.hybridEnabled() ? outcome.denseTopK() : null,
                outcome.hybridEnabled() ? outcome.lexicalTopK() : null,
                outcome.hybridEnabled() ? outcome.fusionStrategy() : null,
                outcome.hybridEnabled() ? outcome.denseCandidateCount() : null,
                outcome.hybridEnabled() ? outcome.lexicalCandidateCount() : null,
                outcome.hybridEnabled() ? outcome.fusedCandidateCount() : null,
                outcome.hybridEnabled() ? outcome.hybridLatencyMs() : null
        );
    }

    private String resolveVectorStoreName() {
        String provider = vectorStoreProperties.getProvider();
        if (ExactCosineVectorStore.PROVIDER.equalsIgnoreCase(provider)) {
            return "ExactCosineVectorStore";
        }
        if (QdrantVectorStore.PROVIDER.equalsIgnoreCase(provider)) {
            return "QdrantVectorStore";
        }
        return vectorStore.getClass().getSimpleName();
    }

    private String contentSnippet(String content) {
        if (content == null) {
            return "";
        }
        if (content.length() <= CONTENT_SNIPPET_MAX_LENGTH) {
            return content;
        }
        return content.substring(0, CONTENT_SNIPPET_MAX_LENGTH);
    }

    private int normalizeTopK(Integer topK) {
        if (topK == null) {
            return DEFAULT_TOP_K;
        }
        if (topK < 1) {
            throw BusinessException.validationError("topK must be greater than or equal to 1");
        }
        if (topK > MAX_TOP_K) {
            throw BusinessException.validationError("topK must be less than or equal to 10");
        }
        return topK;
    }
}
