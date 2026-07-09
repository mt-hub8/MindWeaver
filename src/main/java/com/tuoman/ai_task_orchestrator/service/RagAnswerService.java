package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.dto.RagAnswerRequest;
import com.tuoman.ai_task_orchestrator.dto.RagAnswerResponse;
import com.tuoman.ai_task_orchestrator.dto.RagCitationResponse;
import com.tuoman.ai_task_orchestrator.dto.RagGenerationMetadataResponse;
import com.tuoman.ai_task_orchestrator.dto.RagRetrievalMetadataResponse;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingProvider;
import com.tuoman.ai_task_orchestrator.enums.RetrievalScopeType;
import com.tuoman.ai_task_orchestrator.grounding.AnswerContractMode;
import com.tuoman.ai_task_orchestrator.grounding.AnswerGroundingScore;
import com.tuoman.ai_task_orchestrator.grounding.AnswerGroundingScoreCalculator;
import com.tuoman.ai_task_orchestrator.grounding.CitationVerificationResult;
import com.tuoman.ai_task_orchestrator.grounding.CitationVerificationService;
import com.tuoman.ai_task_orchestrator.grounding.GroundedAnswerContract;
import com.tuoman.ai_task_orchestrator.grounding.GroundedAnswerDiagnostics;
import com.tuoman.ai_task_orchestrator.grounding.GroundedAnswerPromptBuilder;
import com.tuoman.ai_task_orchestrator.grounding.GroundedContextAssembler;
import com.tuoman.ai_task_orchestrator.grounding.GroundedContextBundle;
import com.tuoman.ai_task_orchestrator.grounding.GroundedContextChunk;
import com.tuoman.ai_task_orchestrator.grounding.RefusalDecision;
import com.tuoman.ai_task_orchestrator.grounding.RefusalPolicyService;
import com.tuoman.ai_task_orchestrator.grounding.UnsupportedClaimDetector;
import com.tuoman.ai_task_orchestrator.grounding.UnsupportedClaimReport;
import com.tuoman.ai_task_orchestrator.llm.LlmClient;
import com.tuoman.ai_task_orchestrator.llm.LlmRequest;
import com.tuoman.ai_task_orchestrator.llm.LlmResponse;
import com.tuoman.ai_task_orchestrator.config.RetrievalPipelineProperties;
import com.tuoman.ai_task_orchestrator.retrieval.CollectionAskScope;
import com.tuoman.ai_task_orchestrator.retrieval.RetrievalDiagnostics;
import com.tuoman.ai_task_orchestrator.retrieval.RetrievalScope;
import com.tuoman.ai_task_orchestrator.queryunderstanding.QueryClarificationGuard;
import com.tuoman.ai_task_orchestrator.queryunderstanding.QueryRewriteResult;
import com.tuoman.ai_task_orchestrator.queryunderstanding.QueryRewriteService;
import com.tuoman.ai_task_orchestrator.queryunderstanding.QueryUnderstandingDiagnostics;
import com.tuoman.ai_task_orchestrator.queryunderstanding.QueryUnderstandingResult;
import com.tuoman.ai_task_orchestrator.queryunderstanding.QueryUnderstandingService;
import com.tuoman.ai_task_orchestrator.queryunderstanding.RetrievalRoutingDecision;
import com.tuoman.ai_task_orchestrator.queryunderstanding.RetrievalRoutingPolicyService;
import com.tuoman.ai_task_orchestrator.queryunderstanding.UserSelectedFilters;
import com.tuoman.ai_task_orchestrator.repository.DocumentCollectionRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.rag.quality.RagQualityMode;
import com.tuoman.ai_task_orchestrator.rag.quality.RagQualityService;
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

    private final AppRetrievalService appRetrievalService;

    private final RetrievalPipelineProperties retrievalPipelineProperties;

    private final CollectionScopeService collectionScopeService;

    private final EmbeddingProvider embeddingProvider;

    private final VectorStore vectorStore;

    private final VectorStoreProperties vectorStoreProperties;

    private final RagPromptBuilder ragPromptBuilder;

    private final LlmClient llmClient;

    private final RagQualityService ragQualityService;

    private final QueryUnderstandingService queryUnderstandingService;

    private final QueryRewriteService queryRewriteService;

    private final RetrievalRoutingPolicyService retrievalRoutingPolicyService;

    private final QueryClarificationGuard queryClarificationGuard;

    private final DocumentRepository documentRepository;

    private final DocumentCollectionRepository documentCollectionRepository;

    private final GroundedContextAssembler groundedContextAssembler;

    private final GroundedAnswerPromptBuilder groundedAnswerPromptBuilder;

    private final CitationVerificationService citationVerificationService;

    private final UnsupportedClaimDetector unsupportedClaimDetector;

    private final RefusalPolicyService refusalPolicyService;

    private final AnswerGroundingScoreCalculator answerGroundingScoreCalculator;

    public RagAnswerResponse answer(RagAnswerRequest request) {
        if (request == null) {
            throw BusinessException.validationError("request must not be null");
        }

        int topK = normalizeTopK(request.getTopK());
        CollectionAskScope askScope = resolveAskScope(request.getCollectionId());
        UserSelectedFilters userSelectedFilters = UserSelectedFilters.ofCollection(request.getCollectionId());
        QueryUnderstandingResult understanding = queryUnderstandingService.understand(
                request.getQuery(),
                request.getCollectionId(),
                userSelectedFilters
        );
        QueryRewriteResult rewrite = queryRewriteService.rewrite(understanding);
        RetrievalRoutingDecision routingDecision = retrievalRoutingPolicyService.route(understanding, userSelectedFilters);
        QueryClarificationGuard.GuardResult guardResult = queryClarificationGuard.evaluate(
                understanding,
                routingDecision,
                userSelectedFilters,
                documentRepository.count(),
                documentCollectionRepository.count()
        );
        if (guardResult.clarificationRequired()) {
            RetrievalRoutingDecision guardedDecision = RetrievalRoutingDecision.builder()
                    .strategy(routingDecision.getStrategy())
                    .filter(routingDecision.getFilter())
                    .vectorTopK(routingDecision.getVectorTopK())
                    .keywordTopK(routingDecision.getKeywordTopK())
                    .finalTopK(routingDecision.getFinalTopK())
                    .rerankTopN(routingDecision.getRerankTopN())
                    .contextExpansion(routingDecision.getContextExpansion())
                    .scoringProfile(routingDecision.getScoringProfile())
                    .clarificationRequired(true)
                    .clarificationQuestion(guardResult.clarificationQuestion())
                    .routingReasons(routingDecision.getRoutingReasons())
                    .warnings(routingDecision.getWarnings())
                    .noAnswerRisk(routingDecision.isNoAnswerRisk())
                    .build();
            QueryUnderstandingDiagnostics diagnostics = QueryUnderstandingDiagnostics.from(
                    understanding,
                    rewrite,
                    guardedDecision,
                    String.join(",", guardResult.reasons())
            );
            return withQualityScore(buildClarificationResponse(guardResult.clarificationQuestion(), topK, askScope, diagnostics), request);
        }
        QueryUnderstandingDiagnostics queryDiagnostics = QueryUnderstandingDiagnostics.from(
                understanding,
                rewrite,
                routingDecision,
                null
        );
        if (askScope.shouldSkipRetrieval()) {
            return withQualityScore(buildNoContextResponse(askScope.noContextMessage(), topK, askScope, queryDiagnostics), request);
        }

        RetrievalScope retrievalScope = toRetrievalScope(request.getCollectionId(), askScope);
        AppRetrievalService.UnifiedRetrievalOutcome unifiedOutcome = appRetrievalService.retrieve(
                rewrite.getKeywordQuery() == null || rewrite.getKeywordQuery().isBlank() ? request.getQuery() : rewrite.getKeywordQuery(),
                topK,
                retrievalScope,
                request.getCollectionId(),
                routingDecision
        );
        RagRetrievalOutcome retrievalOutcome = unifiedOutcome.outcome();
        List<RagCitationResponse> citations = toCitations(
                retrievalOutcome,
                retrievalOutcome.rerankEnabled(),
                retrievalOutcome.hybridEnabled()
        );
        RagRetrievalMetadataResponse retrieval = toRetrievalMetadata(
                retrievalOutcome,
                askScope,
                unifiedOutcome.diagnostics(),
                unifiedOutcome.v15Pipeline()
        );
        AnswerContractMode contractMode = AnswerContractMode.defaultMode();
        GroundedAnswerContract contract = new GroundedAnswerContract(contractMode);
        GroundedContextBundle contextBundle = groundedContextAssembler.assemble(
                request.getQuery(),
                retrievalOutcome.chunks(),
                understanding,
                routingDecision,
                retrievalPipelineProperties.getMaxContextChars(),
                contractMode
        );
        citations = enrichCitations(citations, contextBundle);

        if (citations.isEmpty() || contextBundle.isEmpty()) {
            String answer = askScope.collectionId() != null
                    ? "当前所选分组下未检索到可用于回答的文档片段。"
                    : NO_CONTEXT_ANSWER;
            RefusalDecision refusal = refusalPolicyService.decideBeforeGeneration(
                    contextBundle,
                    understanding,
                    routingDecision,
                    contractMode
            );
            GroundedAnswerDiagnostics grounding = buildGroundingDiagnostics(
                    answer,
                    contract,
                    contextBundle,
                    routingDecision,
                    understanding,
                    refusal
            );
            return withQualityScore(buildNoContextResponse(answer, topK, askScope, retrieval, queryDiagnostics, grounding), request);
        }

        RefusalDecision preGenerationRefusal = refusalPolicyService.decideBeforeGeneration(
                contextBundle,
                understanding,
                routingDecision,
                contractMode
        );
        if (preGenerationRefusal.isShouldRefuse()) {
            GroundedAnswerDiagnostics grounding = buildGroundingDiagnostics(
                    preGenerationRefusal.getSuggestedAnswer(),
                    contract,
                    contextBundle,
                    routingDecision,
                    understanding,
                    preGenerationRefusal
            );
            return withQualityScore(buildNoContextResponse(
                    preGenerationRefusal.getSuggestedAnswer(),
                    topK,
                    askScope,
                    retrieval,
                    queryDiagnostics,
                    grounding
            ), request);
        }

        String prompt = groundedAnswerPromptBuilder.buildPrompt(request.getQuery(), contextBundle, contract, understanding);
        if (routingDecision.isNoAnswerRisk()) {
            prompt = prompt + "\n\n如果上下文没有依据，请明确说明未找到依据。";
        }

        LlmRequest llmRequest = new LlmRequest();
        llmRequest.setPrompt(prompt);

        LlmResponse llmResponse = llmClient.generate(llmRequest);
        if (llmResponse == null || !llmResponse.isSuccess() || llmResponse.getContent() == null || llmResponse.getContent().isBlank()) {
            String message = llmResponse == null || llmResponse.getErrorMessage() == null
                    ? "LLM provider error"
                    : llmResponse.getErrorMessage();
            throw BusinessException.llmProviderError(message);
        }

        RagGenerationMetadataResponse generation = new RagGenerationMetadataResponse(
                llmResponse.getProvider(),
                llmResponse.getModel(),
                llmResponse.getProvider(),
                llmResponse.getModel(),
                null,
                null,
                llmResponse.getLatencyMs(),
                llmResponse.getPromptTokenCount(),
                llmResponse.getCompletionTokenCount()
        );
        CitationVerificationResult verification = citationVerificationService.verify(
                llmResponse.getContent(),
                contextBundle,
                routingDecision.getFilter(),
                understanding
        );
        UnsupportedClaimReport unsupportedClaimReport = unsupportedClaimDetector.detect(llmResponse.getContent(), contextBundle);
        RefusalDecision postVerificationRefusal = refusalPolicyService.decideAfterVerification(verification, contractMode);
        AnswerGroundingScore groundingScore = answerGroundingScoreCalculator.calculate(
                contextBundle,
                verification,
                unsupportedClaimReport,
                postVerificationRefusal
        );
        GroundedAnswerDiagnostics grounding = GroundedAnswerDiagnostics.builder()
                .contract(contract)
                .contextBundle(contextBundle)
                .citationVerification(verification)
                .unsupportedClaimReport(unsupportedClaimReport)
                .refusalDecision(postVerificationRefusal)
                .groundingScore(groundingScore)
                .build();
        return withQualityScore(new RagAnswerResponse(
                llmResponse.getContent(),
                citations,
                retrieval,
                generation,
                null,
                queryDiagnostics,
                grounding
        ), request);
    }

    private RagAnswerResponse withQualityScore(RagAnswerResponse response, RagAnswerRequest request) {
        RagQualityMode mode = RagQualityMode.fromRequest(request.getQualityMode());
        return new RagAnswerResponse(
                response.getAnswer(),
                response.getCitations(),
                response.getRetrieval(),
                response.getGeneration(),
                ragQualityService.evaluate(
                        request.getQuery(),
                        response.getAnswer(),
                        response.getCitations(),
                        response.getRetrieval(),
                        response.getGeneration(),
                        mode
                ),
                response.getQueryUnderstanding(),
                response.getGrounding()
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
        return buildNoContextResponse(answer, topK, askScope, toRetrievalMetadataEmpty(topK, askScope), null);
    }

    private RagAnswerResponse buildNoContextResponse(
            String answer,
            int topK,
            CollectionAskScope askScope,
            QueryUnderstandingDiagnostics queryDiagnostics
    ) {
        return buildNoContextResponse(answer, topK, askScope, toRetrievalMetadataEmpty(topK, askScope), queryDiagnostics);
    }

    private RagAnswerResponse buildNoContextResponse(
            String answer,
            int topK,
            CollectionAskScope askScope,
            RagRetrievalMetadataResponse retrieval
    ) {
        return buildNoContextResponse(answer, topK, askScope, retrieval, null);
    }

    private RagAnswerResponse buildNoContextResponse(
            String answer,
            int topK,
            CollectionAskScope askScope,
            RagRetrievalMetadataResponse retrieval,
            QueryUnderstandingDiagnostics queryDiagnostics
    ) {
        return buildNoContextResponse(answer, topK, askScope, retrieval, queryDiagnostics, null);
    }

    private RagAnswerResponse buildNoContextResponse(
            String answer,
            int topK,
            CollectionAskScope askScope,
            RagRetrievalMetadataResponse retrieval,
            QueryUnderstandingDiagnostics queryDiagnostics,
            GroundedAnswerDiagnostics grounding
    ) {
        return new RagAnswerResponse(
                answer,
                List.of(),
                retrieval,
                new RagGenerationMetadataResponse(null, null, null, null, true, NO_CONTEXT_REASON, null, null, null),
                null,
                queryDiagnostics,
                grounding
        );
    }

    private RagAnswerResponse buildClarificationResponse(
            String question,
            int topK,
            CollectionAskScope askScope,
            QueryUnderstandingDiagnostics queryDiagnostics
    ) {
        return new RagAnswerResponse(
                question,
                List.of(),
                toRetrievalMetadataEmpty(topK, askScope),
                new RagGenerationMetadataResponse(null, null, null, null, true, "CLARIFICATION_REQUIRED", null, null, null),
                null,
                queryDiagnostics,
                null
        );
    }

    private GroundedAnswerDiagnostics buildGroundingDiagnostics(
            String answer,
            GroundedAnswerContract contract,
            GroundedContextBundle contextBundle,
            RetrievalRoutingDecision routingDecision,
            QueryUnderstandingResult understanding,
            RefusalDecision refusal
    ) {
        CitationVerificationResult verification = citationVerificationService.verify(
                answer,
                contextBundle,
                routingDecision == null ? null : routingDecision.getFilter(),
                understanding
        );
        UnsupportedClaimReport unsupportedClaimReport = unsupportedClaimDetector.detect(answer, contextBundle);
        AnswerGroundingScore groundingScore = answerGroundingScoreCalculator.calculate(
                contextBundle,
                verification,
                unsupportedClaimReport,
                refusal
        );
        return GroundedAnswerDiagnostics.builder()
                .contract(contract)
                .contextBundle(contextBundle)
                .citationVerification(verification)
                .unsupportedClaimReport(unsupportedClaimReport)
                .refusalDecision(refusal)
                .groundingScore(groundingScore)
                .build();
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

    private List<RagCitationResponse> enrichCitations(List<RagCitationResponse> citations, GroundedContextBundle contextBundle) {
        if (citations == null || citations.isEmpty() || contextBundle == null || contextBundle.getChunks() == null) {
            return citations == null ? List.of() : citations;
        }
        return citations.stream()
                .map(citation -> {
                    GroundedContextChunk contextChunk = contextBundle.getChunks().stream()
                            .filter(chunk -> citation.getChunkId() != null && citation.getChunkId().equals(chunk.getChunkId()))
                            .findFirst()
                            .orElse(null);
                    if (contextChunk == null) {
                        return citation;
                    }
                    return new RagCitationResponse(
                            citation.getSourceIndex(),
                            citation.getDocumentId(),
                            citation.getChunkId(),
                            citation.getScore(),
                            citation.getContentSnippet(),
                            citation.getOriginalRank(),
                            citation.getRerankedRank(),
                            citation.getOriginalScore(),
                            citation.getRerankScore(),
                            citation.getDenseRank(),
                            citation.getLexicalRank(),
                            citation.getDenseScore(),
                            citation.getLexicalScore(),
                            citation.getFusionScore(),
                            citation.getDenseHit(),
                            citation.getLexicalHit(),
                            contextChunk.getDocumentTitle(),
                            contextChunk.getSectionPath(),
                            contextChunk.getVersion(),
                            contextChunk.getCitationKey(),
                            null
                    );
                })
                .toList();
    }

    private RagRetrievalMetadataResponse toRetrievalMetadata(
            RagRetrievalOutcome outcome,
            CollectionAskScope askScope,
            RetrievalDiagnostics diagnostics,
            boolean v15Pipeline
    ) {
        String filterMode = diagnostics != null && diagnostics.getFilterMode() != null
                ? diagnostics.getFilterMode()
                : "APPLICATION_SIDE";
        String contextExpansion = diagnostics != null && diagnostics.getContextExpansion() != null
                ? diagnostics.getContextExpansion()
                : retrievalPipelineProperties.getContextExpansion() == null
                ? "NONE"
                : retrievalPipelineProperties.getContextExpansion().name();
        String strategy = diagnostics != null && diagnostics.getStrategy() != null
                ? diagnostics.getStrategy()
                : resolveRetrievalStrategy(outcome);
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
                outcome.chunks().size(),
                strategy,
                filterMode,
                contextExpansion
        );
    }

    private String resolveRetrievalStrategy(RagRetrievalOutcome outcome) {
        if (appRetrievalService.useV15Pipeline()) {
            if (retrievalPipelineProperties.isRerankEnabled()) {
                return "HYBRID_RRF_RERANK";
            }
            return "HYBRID_RRF";
        }
        if (outcome.hybridEnabled() && outcome.rerankEnabled()) {
            return "HYBRID_RRF_RERANK";
        }
        if (outcome.hybridEnabled()) {
            return "HYBRID_RRF";
        }
        return "VECTOR_ONLY";
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
                0,
                "VECTOR_ONLY",
                "APPLICATION_SIDE",
                "ADJACENT"
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
