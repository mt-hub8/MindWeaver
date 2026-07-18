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
import com.tuoman.ai_task_orchestrator.memory.MemoryContextAssembler;
import com.tuoman.ai_task_orchestrator.memory.MemoryContextBundle;
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
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Service;

import java.util.List;

/**
 * RAG Answer 主编排服务。
 *
 * V2.3/V2.7 打通了“检索 -> prompt -> LLM -> citation”的早期 RAG Answer API，
 * 后续 V17/V18 在同一主链路上接入 Query Understanding、Retrieval Routing、
 * Grounded Answer 和 Citation Verification。
 *
 * 该类负责把一次用户提问串成完整可信回答链路：
 * Query Understanding -> Retrieval Routing -> Retrieval -> Grounded Context
 * -> LLM generation -> Citation Verification -> RagQualityScore。
 *
 * 关键约束：
 * - collection / version / status filter 不能在链路中丢失，否则会造成跨知识库或错误版本污染；
 * - citations 必须来自 final context，grounded answer 不能引用不在 context 中的 chunk；
 * - no context / low confidence 时拒答是可信 RAG 的正常分支，不应视为异常；
 * - quality score 和 grounding score 只提供诊断，不应反向修改检索结果。
 */
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

    @Autowired(required = false)
    private MemoryContextAssembler memoryContextAssembler;

    public RagAnswerResponse answer(RagAnswerRequest request) {
        if (request == null) {
            throw BusinessException.validationError("request must not be null");
        }

        // 阶段 1：参数归一化。
        // topK 在入口处收敛到安全范围，避免后续检索、rerank、上下文组装使用不一致的数量边界。
        int topK = normalizeTopK(request.getTopK());

        // 阶段 2：collection scope 解析。
        // 用户显式选择的 collection 是硬约束，后续 Query Understanding 和 Retrieval Routing 只能补充，
        // 不能把这个范围丢掉或扩大成盲目全库搜索。
        CollectionAskScope askScope = resolveAskScope(request.getCollectionId());
        UserSelectedFilters userSelectedFilters = UserSelectedFilters.ofCollection(request.getCollectionId());

        // 阶段 3：Query Understanding。
        // 自然语言问题在这里被转换为 queryType、versionHint、statusHint、codeSymbols 等结构化信号，
        // 后续 routing 会基于这些信号决定 filter、rerank 和 context expansion。
        QueryUnderstandingResult understanding = queryUnderstandingService.understand(
                request.getQuery(),
                request.getCollectionId(),
                userSelectedFilters
        );
        QueryRewriteResult rewrite = queryRewriteService.rewrite(understanding);

        // Memory 与知识库资料是两条独立上下文通道。记忆只影响偏好、约束和项目背景，
        // 不会进入 GroundedContextBundle，也不会生成或替代知识库 citation。
        MemoryContextBundle memoryContext = assembleMemoryContext(request);

        // 阶段 4：Retrieval Routing。
        // routingDecision 是检索策略和 metadata pre-filter 的统一承载点。
        // collection / version / status filter 必须沿着后续链路传递，避免召回 TRASHED、PURGED 或错误版本内容。
        RetrievalRoutingDecision routingDecision = retrievalRoutingPolicyService.route(understanding, userSelectedFilters);

        // 阶段 5：Clarification Guard。
        // clarificationRequired=true 表示问题或范围还不够确定，此时不能继续盲目全库搜索；
        // 返回澄清问题比生成低置信答案更符合可信 RAG 的边界。
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
            return withQualityScore(
                    buildClarificationResponse(guardResult.clarificationQuestion(), topK, askScope, diagnostics),
                    request,
                    memoryContext
            );
        }
        QueryUnderstandingDiagnostics queryDiagnostics = QueryUnderstandingDiagnostics.from(
                understanding,
                rewrite,
                routingDecision,
                null
        );
        if (askScope.shouldSkipRetrieval()) {
            return withQualityScore(
                    buildNoContextResponse(askScope.noContextMessage(), topK, askScope, queryDiagnostics),
                    request,
                    memoryContext
            );
        }

        // 阶段 6：AppRetrievalService 检索。
        // 统一检索入口会屏蔽 legacy / hybrid pipeline 差异，但不能绕过 routingDecision 中的 RetrievalFilter。
        // 返回的 chunks 既要用于 citations，也要能被 GroundedContextAssembler 组装成 final context。
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

        // 阶段 7：GroundedContextBundle 组装。
        // final context 是答案可引用证据的唯一来源；TRASHED / PURGED 文档不能进入这里。
        // 后续 citationKey 会绑定到 final context，避免模型引用未提供给它的 chunk。
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

        // 阶段 8：无上下文分支。
        // 没有可用 context 时拒答不是失败，而是明确告诉调用方当前知识库无法支撑回答。
        // grounding / quality diagnostics 仍会返回，便于定位是检索为空、范围过窄还是证据不足。
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
            return withQualityScore(
                    buildNoContextResponse(answer, topK, askScope, retrieval, queryDiagnostics, grounding),
                    request,
                    memoryContext
            );
        }

        // 阶段 9：RefusalPolicy 生成前拒答。
        // low confidence、版本缺失或 STRICT 证据不足时，拒答优先于让 LLM 猜测。
        // 这条分支仍保留 grounding diagnostics，用于说明为什么没有进入生成阶段。
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
            ), request, memoryContext);
        }

        // 阶段 10：GroundedAnswerPromptBuilder 构造 prompt。
        // prompt 只允许使用 final context 中的证据；citations 必须能回到 contextBundle 中的 citationKey。
        String prompt = groundedAnswerPromptBuilder.buildPrompt(request.getQuery(), contextBundle, contract, understanding);
        if (memoryContextAssembler != null) {
            prompt = memoryContextAssembler.toPromptSection(memoryContext)
                    + "\n【知识库资料】\n"
                    + prompt;
        }
        if (routingDecision.isNoAnswerRisk()) {
            prompt = prompt + "\n\n如果上下文没有依据，请明确说明未找到依据。";
        }

        LlmRequest llmRequest = new LlmRequest();
        llmRequest.setPrompt(prompt);

        // 阶段 11：LLM generate。
        // LLM 只负责在受约束 prompt 下生成文本，不负责扩大检索范围或修正 filter。
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
        // 阶段 12：CitationVerification。
        // 验证答案中的引用是否存在于 final context，并检查引用是否仍满足 routing filter。
        // grounded answer 不能引用 context 外的 chunk，否则引用校验和用户信任都会失效。
        CitationVerificationResult verification = citationVerificationService.verify(
                llmResponse.getContent(),
                contextBundle,
                routingDecision.getFilter(),
                understanding
        );

        // 阶段 13：UnsupportedClaimDetector。
        // 这里检查关键 claim 是否缺 citation，或 citation 是否无法支撑符号、版本、数字等强约束信息。
        UnsupportedClaimReport unsupportedClaimReport = unsupportedClaimDetector.detect(llmResponse.getContent(), contextBundle);
        RefusalDecision postVerificationRefusal = refusalPolicyService.decideAfterVerification(verification, contractMode);

        // 阶段 14：AnswerGroundingScore。
        // grounding score 是对答案可信度的诊断，不回写检索结果，也不改变已返回的 citations。
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

        // 阶段 15：response 组装。
        // 最终响应同时返回答案、citations、retrieval metadata、generation metadata 和 diagnostics，
        // 便于前端展示，也便于后续排查检索污染、引用失败或质量评分偏低的问题。
        return withQualityScore(new RagAnswerResponse(
                llmResponse.getContent(),
                citations,
                retrieval,
                generation,
                null,
                queryDiagnostics,
                grounding,
                memoryContext
        ), request, memoryContext);
    }

    private RagAnswerResponse withQualityScore(
            RagAnswerResponse response,
            RagAnswerRequest request,
            MemoryContextBundle memoryContext
    ) {
        // RagQualityScore 是最后追加的诊断视图，只读取 answer / citations / metadata。
        // 它不应反向修改 retrieval outcome 或 grounding outcome，避免评分逻辑影响主链路结果。
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
                response.getGrounding(),
                memoryContext
        );
    }

    private MemoryContextBundle assembleMemoryContext(RagAnswerRequest request) {
        if (memoryContextAssembler == null) {
            return MemoryContextBundle.empty(null);
        }
        return memoryContextAssembler.assemble(
                request.getQuery(),
                request.getProjectId(),
                request.getAgentProfileId(),
                request.getTaskId(),
                request.getMemoryScopes(),
                request.getMemoryLimit()
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
