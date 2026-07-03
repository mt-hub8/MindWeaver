package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.dto.RagAnswerRequest;
import com.tuoman.ai_task_orchestrator.dto.RagAnswerResponse;
import com.tuoman.ai_task_orchestrator.dto.RagCitationResponse;
import com.tuoman.ai_task_orchestrator.dto.RagGenerationMetadataResponse;
import com.tuoman.ai_task_orchestrator.dto.RagRetrievalMetadataResponse;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingProvider;
import com.tuoman.ai_task_orchestrator.enums.RetrievalScopeType;
import com.tuoman.ai_task_orchestrator.llm.LlmClient;
import com.tuoman.ai_task_orchestrator.llm.LlmRequest;
import com.tuoman.ai_task_orchestrator.llm.LlmResponse;
import com.tuoman.ai_task_orchestrator.retrieval.CollectionAskScope;
import com.tuoman.ai_task_orchestrator.retrieval.RetrievalScope;
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

    private final RagTwoStageRetrievalService ragTwoStageRetrievalService;

    private final CollectionScopeService collectionScopeService;

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
        CollectionAskScope askScope = resolveAskScope(request.getCollectionId());
        if (askScope.shouldSkipRetrieval()) {
            return buildNoContextResponse(askScope.noContextMessage(), topK, askScope);
        }

        RetrievalScope retrievalScope = toRetrievalScope(request.getCollectionId(), askScope);
        RagRetrievalOutcome retrievalOutcome = ragTwoStageRetrievalService.retrieve(
                request.getQuery(),
                topK,
                retrievalScope
        );
        List<RagCitationResponse> citations = toCitations(
                retrievalOutcome,
                retrievalOutcome.rerankEnabled(),
                retrievalOutcome.hybridEnabled()
        );
        RagRetrievalMetadataResponse retrieval = toRetrievalMetadata(retrievalOutcome, askScope);

        if (citations.isEmpty()) {
            String answer = askScope.collectionId() != null
                    ? "当前所选分组下未检索到可用于回答的文档片段。"
                    : NO_CONTEXT_ANSWER;
            return buildNoContextResponse(answer, topK, askScope, retrieval);
        }

        String prompt = ragPromptBuilder.buildPrompt(request.getQuery(), citations);

        LlmRequest llmRequest = new LlmRequest();
        llmRequest.setPrompt(prompt);

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
                        llmResponse.getProvider(),
                        llmResponse.getModel(),
                        null,
                        null
                )
        );
    }

    private CollectionAskScope resolveAskScope(Long collectionId) {
        if (collectionId == null) {
            return CollectionAskScope.notApplicable();
        }
        return collectionScopeService.resolveForAsk(collectionId);
    }

    private RetrievalScope toRetrievalScope(Long collectionId, CollectionAskScope askScope) {
        if (collectionId == null) {
            return RetrievalScope.allDocuments();
        }
        return RetrievalScope.collection(
                askScope.collectionId(),
                askScope.collectionName(),
                askScope.askableDocumentIds()
        );
    }

    private RagAnswerResponse buildNoContextResponse(String answer, int topK, CollectionAskScope askScope) {
        return buildNoContextResponse(answer, topK, askScope, toRetrievalMetadataEmpty(topK, askScope));
    }

    private RagAnswerResponse buildNoContextResponse(
            String answer,
            int topK,
            CollectionAskScope askScope,
            RagRetrievalMetadataResponse retrieval
    ) {
        return new RagAnswerResponse(
                answer,
                List.of(),
                retrieval,
                new RagGenerationMetadataResponse(null, null, null, null, true, NO_CONTEXT_REASON)
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

    private RagRetrievalMetadataResponse toRetrievalMetadata(
            RagRetrievalOutcome outcome,
            CollectionAskScope askScope
    ) {
        return new RagRetrievalMetadataResponse(
                outcome.finalTopK(),
                outcome.chunks().size(),
                embeddingProvider.runtimeProvider(),
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
                outcome.hybridEnabled() ? outcome.hybridLatencyMs() : null,
                askScope.collectionId() == null
                        ? RetrievalScopeType.ALL_DOCUMENTS.name()
                        : RetrievalScopeType.COLLECTION.name(),
                askScope.collectionId(),
                askScope.collectionName(),
                null,
                null,
                outcome.chunks().size()
        );
    }

    private RagRetrievalMetadataResponse toRetrievalMetadataEmpty(int topK, CollectionAskScope askScope) {
        return new RagRetrievalMetadataResponse(
                topK,
                0,
                embeddingProvider.runtimeProvider(),
                embeddingProvider.model(),
                embeddingProvider.dimension(),
                resolveVectorStoreName(),
                null,
                null,
                null,
                topK,
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                askScope.collectionId() == null
                        ? RetrievalScopeType.ALL_DOCUMENTS.name()
                        : RetrievalScopeType.COLLECTION.name(),
                askScope.collectionId(),
                askScope.collectionName(),
                null,
                null,
                0
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
