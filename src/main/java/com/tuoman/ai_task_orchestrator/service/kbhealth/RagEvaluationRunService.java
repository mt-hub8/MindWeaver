package com.tuoman.ai_task_orchestrator.service.kbhealth;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.dto.RagAnswerRequest;
import com.tuoman.ai_task_orchestrator.dto.RagAnswerResponse;
import com.tuoman.ai_task_orchestrator.dto.RagCitationResponse;
import com.tuoman.ai_task_orchestrator.dto.kbhealth.CompareRagEvaluationRunsRequest;
import com.tuoman.ai_task_orchestrator.dto.kbhealth.CompareRagEvaluationRunsResponse;
import com.tuoman.ai_task_orchestrator.dto.kbhealth.CreateRagEvaluationRunRequest;
import com.tuoman.ai_task_orchestrator.dto.kbhealth.RagEvaluationCaseResultResponse;
import com.tuoman.ai_task_orchestrator.dto.kbhealth.RagEvaluationRunResponse;
import com.tuoman.ai_task_orchestrator.entity.RagEvaluationCaseEntity;
import com.tuoman.ai_task_orchestrator.entity.RagEvaluationCaseResultEntity;
import com.tuoman.ai_task_orchestrator.entity.RagEvaluationDatasetEntity;
import com.tuoman.ai_task_orchestrator.entity.RagEvaluationRunEntity;
import com.tuoman.ai_task_orchestrator.kbhealth.EvaluationRetrievedChunk;
import com.tuoman.ai_task_orchestrator.kbhealth.HealthMetricValue;
import com.tuoman.ai_task_orchestrator.kbhealth.JsonFieldCodec;
import com.tuoman.ai_task_orchestrator.kbhealth.KnowledgeBaseDiagnosisService;
import com.tuoman.ai_task_orchestrator.kbhealth.RagEvaluationRetrievalStrategy;
import com.tuoman.ai_task_orchestrator.kbhealth.RagEvaluationRunStatus;
import com.tuoman.ai_task_orchestrator.kbhealth.RagHealthDisplayTexts;
import com.tuoman.ai_task_orchestrator.kbhealth.RagHealthGenerationMetricsCalculator;
import com.tuoman.ai_task_orchestrator.kbhealth.RagHealthQualityScoreCalculator;
import com.tuoman.ai_task_orchestrator.kbhealth.RagHealthRetrievalMetricsCalculator;
import com.tuoman.ai_task_orchestrator.kbhealth.RagHealthScoringProfile;
import com.tuoman.ai_task_orchestrator.kbhealth.RagQualityVetoRuleService;
import com.tuoman.ai_task_orchestrator.kbhealth.RetrievalStrategyRunner;
import com.tuoman.ai_task_orchestrator.grounding.GroundedAnswerDiagnostics;
import com.tuoman.ai_task_orchestrator.queryunderstanding.QueryUnderstandingMetricsService;
import com.tuoman.ai_task_orchestrator.queryunderstanding.QueryUnderstandingResult;
import com.tuoman.ai_task_orchestrator.queryunderstanding.QueryUnderstandingService;
import com.tuoman.ai_task_orchestrator.queryunderstanding.RetrievalRoutingDecision;
import com.tuoman.ai_task_orchestrator.queryunderstanding.RetrievalRoutingPolicyService;
import com.tuoman.ai_task_orchestrator.queryunderstanding.UserSelectedFilters;
import com.tuoman.ai_task_orchestrator.repository.RagEvaluationCaseRepository;
import com.tuoman.ai_task_orchestrator.repository.RagEvaluationCaseResultRepository;
import com.tuoman.ai_task_orchestrator.repository.RagEvaluationDatasetRepository;
import com.tuoman.ai_task_orchestrator.repository.RagEvaluationRunRepository;
import com.tuoman.ai_task_orchestrator.service.RagAnswerService;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.LocalDateTime;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.stream.Collectors;

/**
 * V14 RAG Evaluation Run 服务。
 *
 * Run 将某个 Dataset 中的 enabled Case 按指定 retrieval strategy 和 scoring profile 执行，
 * 为每个 Case 生成 CaseResult，再汇总为 Knowledge Health 报告。
 *
 * 关键不变量：Run Compare 只有在同一 Dataset/Case 上比较 baseline 与 candidate 才有意义；
 * 指标缺失必须保留 UNKNOWN，不能编造成 0 或 1。
 */
@Service
@RequiredArgsConstructor
public class RagEvaluationRunService {

    private static final int DEFAULT_TOP_K = 5;

    private final RagEvaluationRunRepository runRepository;

    private final RagEvaluationDatasetRepository datasetRepository;

    private final RagEvaluationCaseRepository caseRepository;

    private final RagEvaluationCaseResultRepository caseResultRepository;

    private final RetrievalStrategyRunner retrievalStrategyRunner;

    private final RagHealthRetrievalMetricsCalculator retrievalMetricsCalculator;

    private final RagHealthGenerationMetricsCalculator generationMetricsCalculator;

    private final RagHealthQualityScoreCalculator qualityScoreCalculator;

    private final RagQualityVetoRuleService vetoRuleService;

    private final KnowledgeBaseDiagnosisService diagnosisService;

    private final RagAnswerService ragAnswerService;

    private final ObjectMapper objectMapper;

    private final QueryUnderstandingService queryUnderstandingService;

    private final RetrievalRoutingPolicyService retrievalRoutingPolicyService;

    private final QueryUnderstandingMetricsService queryUnderstandingMetricsService;

    @Transactional
    public RagEvaluationRunResponse createAndExecuteRun(CreateRagEvaluationRunRequest request) {
        // 状态机：RUNNING 表示 case 正在执行；全部 case 结束后才进入 COMPLETED/FAILED。
        // 单个 case 失败会记录 CaseResult，便于定位失败样本。
        if (request == null || request.getDatasetId() == null) {
            throw BusinessException.validationError("datasetId is required");
        }
        RagEvaluationDatasetEntity dataset = datasetRepository.findById(request.getDatasetId())
                .orElseThrow(() -> BusinessException.invalidRequest("dataset not found"));
        List<RagEvaluationCaseEntity> cases = caseRepository.findByDatasetIdAndEnabledTrueOrderByIdAsc(dataset.getId());
        if (cases.isEmpty()) {
            throw BusinessException.validationError("dataset has no enabled cases");
        }

        RagEvaluationRunEntity run = new RagEvaluationRunEntity();
        run.setDatasetId(dataset.getId());
        run.setName(request.getName() == null || request.getName().isBlank()
                ? "评测运行 " + LocalDateTime.now()
                : request.getName().trim());
        run.setStatus(RagEvaluationRunStatus.RUNNING);
        run.setStrategy(request.getStrategy() == null ? RagEvaluationRetrievalStrategy.VECTOR_ONLY : request.getStrategy());
        run.setScoringProfile(request.getScoringProfile() == null ? RagHealthScoringProfile.BALANCED : request.getScoringProfile());
        run.setTopK(request.getTopK() == null ? DEFAULT_TOP_K : request.getTopK());
        run.setRetrievalTopK(request.getRetrievalTopK());
        run.setRerankTopN(request.getRerankTopN());
        run.setCollectionId(request.getCollectionId());
        Map<String, Object> metadataFilter = request.getMetadataFilter() == null
                ? new HashMap<>()
                : new HashMap<>(request.getMetadataFilter());
        metadataFilter.put("_enableQueryUnderstanding", Boolean.TRUE.equals(request.getEnableQueryUnderstanding()));
        run.setMetadataFilterJson(JsonFieldCodec.write(metadataFilter));
        run.setExecuteGeneration(Boolean.TRUE.equals(request.getExecuteGeneration()));
        run.setStartedAt(LocalDateTime.now());
        run.setTotalCases(cases.size());
        run.setCompletedCases(0);
        run.setFailedCases(0);
        run = runRepository.save(run);

        List<HealthMetricValue> aggregatedRetrieval = new ArrayList<>();
        List<HealthMetricValue> aggregatedGeneration = new ArrayList<>();
        int caseScoreSum = 0;
        int scoredCases = 0;
        int failed = 0;
        int completed = 0;

        Map<String, Object> runMetadataFilter = JsonFieldCodec.readMap(run.getMetadataFilterJson());

        for (RagEvaluationCaseEntity evalCase : cases) {
            if (run.getStatus() == RagEvaluationRunStatus.CANCELED) {
                break;
            }
            try {
                RagEvaluationCaseResultEntity result = executeCase(run, evalCase, runMetadataFilter);
                caseResultRepository.save(result);
                completed++;
                if (result.getQualityScore() != null) {
                    caseScoreSum += result.getQualityScore();
                    scoredCases++;
                }
                aggregatedRetrieval.addAll(readMetrics(result.getRetrievalMetricsJson()));
                aggregatedGeneration.addAll(readMetrics(result.getGenerationMetricsJson()));
            } catch (Exception exception) {
                failed++;
                RagEvaluationCaseResultEntity failedResult = new RagEvaluationCaseResultEntity();
                failedResult.setRunId(run.getId());
                failedResult.setCaseRefId(evalCase.getId());
                failedResult.setCaseId(evalCase.getCaseId());
                failedResult.setQuery(evalCase.getQuery());
                failedResult.setStrategy(run.getStrategy());
                failedResult.setTopK(run.getTopK());
                failedResult.setErrorCode("CASE_FAILED");
                failedResult.setErrorMessage(exception.getMessage());
                caseResultRepository.save(failedResult);
            }
        }

        List<HealthMetricValue> avgRetrieval = averageMetrics(aggregatedRetrieval);
        List<HealthMetricValue> avgGeneration = averageMetrics(aggregatedGeneration);
        List<HealthMetricValue> allMetrics = new ArrayList<>();
        allMetrics.addAll(avgRetrieval);
        allMetrics.addAll(avgGeneration);

        RagHealthQualityScoreCalculator.RagQualityScoreResult scoreResult =
                qualityScoreCalculator.score(allMetrics, run.getScoringProfile());
        RagQualityVetoRuleService.VetoResult vetoResult = vetoRuleService.apply(scoreResult.getOverallScore(), allMetrics);
        KnowledgeBaseDiagnosisService.DiagnosisResult diagnosis = diagnosisService.diagnose(allMetrics);

        run.setCompletedCases(completed);
        run.setFailedCases(failed);
        run.setCompletedAt(LocalDateTime.now());
        run.setStatus(failed == cases.size() ? RagEvaluationRunStatus.FAILED : RagEvaluationRunStatus.COMPLETED);
        run.setOverallScore(vetoResult.getFinalScore());
        run.setSummaryJson(JsonFieldCodec.write(buildSummary(run, scoreResult, vetoResult, avgRetrieval, avgGeneration)));
        run.setDiagnosisJson(JsonFieldCodec.write(diagnosis));
        runRepository.save(run);

        return toRunResponse(run);
    }

    @Transactional(readOnly = true)
    public List<RagEvaluationRunResponse> listRuns() {
        return runRepository.findAllByOrderByCreatedAtDesc().stream().map(this::toRunResponse).toList();
    }

    @Transactional(readOnly = true)
    public RagEvaluationRunResponse getRun(Long runId) {
        return toRunResponse(findRunOrThrow(runId));
    }

    @Transactional(readOnly = true)
    public List<RagEvaluationCaseResultResponse> listCaseResults(Long runId) {
        findRunOrThrow(runId);
        return caseResultRepository.findByRunIdOrderByIdAsc(runId).stream()
                .map(this::toCaseResultResponse)
                .toList();
    }

    @Transactional
    public RagEvaluationRunResponse cancelRun(Long runId) {
        RagEvaluationRunEntity run = findRunOrThrow(runId);
        run.setStatus(RagEvaluationRunStatus.CANCELED);
        run.setCompletedAt(LocalDateTime.now());
        return toRunResponse(runRepository.save(run));
    }

    @Transactional(readOnly = true)
    public CompareRagEvaluationRunsResponse compareRuns(CompareRagEvaluationRunsRequest request) {
        if (request == null || request.getBaselineRunId() == null || request.getCandidateRunId() == null) {
            throw BusinessException.validationError("baselineRunId and candidateRunId are required");
        }
        RagEvaluationRunEntity baseline = findRunOrThrow(request.getBaselineRunId());
        RagEvaluationRunEntity candidate = findRunOrThrow(request.getCandidateRunId());
        Map<String, Object> baselineSummary = JsonFieldCodec.readMap(baseline.getSummaryJson());
        Map<String, Object> candidateSummary = JsonFieldCodec.readMap(candidate.getSummaryJson());

        Map<String, Double> baselineMetrics = metricMap(baselineSummary);
        Map<String, Double> candidateMetrics = metricMap(candidateSummary);
        Map<String, Double> deltas = new LinkedHashMap<>();
        List<String> improved = new ArrayList<>();
        List<String> regressed = new ArrayList<>();

        for (String key : baselineMetrics.keySet()) {
            if (!candidateMetrics.containsKey(key)) {
                continue;
            }
            double delta = candidateMetrics.get(key) - baselineMetrics.get(key);
            deltas.put(key, delta);
            if (key.contains("LEAK")) {
                if (delta < 0) {
                    improved.add(key);
                } else if (delta > 0) {
                    regressed.add(key);
                }
            } else if (delta > 0.01) {
                improved.add(key);
            } else if (delta < -0.01) {
                regressed.add(key);
            }
        }

        int baselineScore = baseline.getOverallScore() == null ? 0 : baseline.getOverallScore();
        int candidateScore = candidate.getOverallScore() == null ? 0 : candidate.getOverallScore();
        int deltaScore = candidateScore - baselineScore;
        String summary = deltaScore > 0
                ? "候选策略整体评分提升 " + deltaScore + " 分。"
                : deltaScore < 0
                ? "候选策略整体评分下降 " + Math.abs(deltaScore) + " 分。"
                : "整体评分无显著变化。";

        List<String> suggestions = new ArrayList<>();
        if (improved.contains("RECALL_AT_K") && regressed.contains("latencyMs")) {
            suggestions.add("检索质量提升明显，但延迟增加，需要根据场景选择是否启用 rerank。");
        }
        if (deltaScore > 0) {
            suggestions.add("可优先采用候选检索策略作为默认配置。");
        }

        return new CompareRagEvaluationRunsResponse(
                baselineScore,
                candidateScore,
                deltaScore,
                deltas,
                improved,
                regressed,
                summary,
                suggestions
        );
    }

    private RagEvaluationCaseResultEntity executeCase(
            RagEvaluationRunEntity run,
            RagEvaluationCaseEntity evalCase,
            Map<String, Object> runMetadataFilter
    ) {
        long startedAt = System.nanoTime();
        boolean enableQueryUnderstanding = Boolean.TRUE.equals(runMetadataFilter.get("_enableQueryUnderstanding"));
        Map<String, Object> effectiveRunFilter = new HashMap<>(runMetadataFilter);
        effectiveRunFilter.remove("_enableQueryUnderstanding");
        QueryUnderstandingResult understanding = null;
        RetrievalRoutingDecision routingDecision = null;
        RagEvaluationRetrievalStrategy strategy = run.getStrategy();
        Long collectionId = run.getCollectionId();
        if (enableQueryUnderstanding) {
            UserSelectedFilters selected = UserSelectedFilters.ofCollection(
                    run.getCollectionId() != null ? run.getCollectionId() : evalCase.getCollectionId()
            );
            understanding = queryUnderstandingService.understand(evalCase.getQuery(), selected.getCollectionId(), selected);
            routingDecision = retrievalRoutingPolicyService.route(understanding, selected);
            strategy = toEvaluationStrategy(routingDecision);
            if (collectionId == null && routingDecision.getFilter() != null) {
                collectionId = routingDecision.getFilter().getCollectionId();
            }
            if (routingDecision.getFilter() != null && routingDecision.getFilter().getVersion() != null) {
                effectiveRunFilter.put("version", routingDecision.getFilter().getVersion());
            }
        }
        RetrievalStrategyRunner.RetrievalRunOutcome retrievalOutcome = retrievalStrategyRunner.retrieve(
                evalCase,
                strategy,
                run.getTopK(),
                run.getRetrievalTopK() == null ? run.getTopK() * 2 : run.getRetrievalTopK(),
                run.getRerankTopN(),
                collectionId,
                effectiveRunFilter
        );

        String answer = null;
        List<String> citations = List.of();
        GroundedAnswerDiagnostics grounding = null;
        if (Boolean.TRUE.equals(run.getExecuteGeneration())) {
            RagAnswerRequest answerRequest = new RagAnswerRequest();
            answerRequest.setQuery(evalCase.getQuery());
            answerRequest.setTopK(run.getTopK());
            answerRequest.setCollectionId(collectionId != null ? collectionId : evalCase.getCollectionId());
            RagAnswerResponse answerResponse = ragAnswerService.answer(answerRequest);
            answer = answerResponse.getAnswer();
            grounding = answerResponse.getGrounding();
            citations = answerResponse.getCitations() == null
                    ? List.of()
                    : answerResponse.getCitations().stream()
                    .map(RagCitationResponse::getChunkId)
                    .map(String::valueOf)
                    .toList();
        } else {
            answer = buildHeuristicAnswer(retrievalOutcome.chunks());
            citations = retrievalOutcome.chunks().stream()
                    .map(chunk -> chunk.getChunkId() == null ? null : String.valueOf(chunk.getChunkId()))
                    .filter(id -> id != null)
                    .limit(3)
                    .toList();
        }

        List<HealthMetricValue> retrievalMetrics = retrievalMetricsCalculator.calculate(
                evalCase,
                retrievalOutcome.chunks(),
                run.getTopK()
        );
        if (enableQueryUnderstanding && understanding != null) {
            retrievalMetrics = new ArrayList<>(retrievalMetrics);
            retrievalMetrics.addAll(queryUnderstandingMetricsService.calculate(evalCase, understanding, routingDecision));
        }
        List<HealthMetricValue> generationMetrics = generationMetricsCalculator.calculate(
                evalCase,
                answer,
                citations,
                retrievalOutcome.chunks()
        );
        if (grounding != null) {
            generationMetrics = new ArrayList<>(generationMetrics);
            generationMetrics.addAll(groundingMetrics(grounding));
        }

        List<HealthMetricValue> allMetrics = new ArrayList<>();
        allMetrics.addAll(retrievalMetrics);
        allMetrics.addAll(generationMetrics);
        RagHealthQualityScoreCalculator.RagQualityScoreResult scoreResult =
                qualityScoreCalculator.score(allMetrics, run.getScoringProfile());
        RagQualityVetoRuleService.VetoResult vetoResult = vetoRuleService.apply(scoreResult.getOverallScore(), allMetrics);
        KnowledgeBaseDiagnosisService.DiagnosisResult diagnosis = diagnosisService.diagnose(allMetrics);

        long latencyMs = (System.nanoTime() - startedAt) / 1_000_000;

        RagEvaluationCaseResultEntity result = new RagEvaluationCaseResultEntity();
        result.setRunId(run.getId());
        result.setCaseRefId(evalCase.getId());
        result.setCaseId(evalCase.getCaseId());
        result.setQuery(evalCase.getQuery());
        result.setStrategy(strategy);
        result.setTopK(run.getTopK());
        result.setRetrievedChunksJson(JsonFieldCodec.write(retrievalOutcome.chunks()));
        result.setGeneratedAnswer(answer);
        result.setCitationsJson(JsonFieldCodec.write(citations));
        if (grounding != null) {
            result.setContextBundleJson(JsonFieldCodec.write(grounding.getContextBundle()));
            result.setCitationVerificationJson(JsonFieldCodec.write(grounding.getCitationVerification()));
            result.setUnsupportedClaimReportJson(JsonFieldCodec.write(grounding.getUnsupportedClaimReport()));
            result.setRefusalDecisionJson(JsonFieldCodec.write(grounding.getRefusalDecision()));
            result.setGroundingScoreJson(JsonFieldCodec.write(grounding.getGroundingScore()));
        }
        result.setRetrievalMetricsJson(JsonFieldCodec.write(retrievalMetrics));
        result.setGenerationMetricsJson(JsonFieldCodec.write(generationMetrics));
        result.setQualityScore(vetoResult.getFinalScore());
        result.setDiagnosisJson(JsonFieldCodec.write(diagnosis));
        result.setLatencyMs(latencyMs);
        return result;
    }

    private List<HealthMetricValue> groundingMetrics(GroundedAnswerDiagnostics grounding) {
        List<HealthMetricValue> metrics = new ArrayList<>();
        if (grounding.getCitationVerification() != null) {
            metrics.add(HealthMetricValue.of(
                    "GROUNDED_CITATION_ACCURACY",
                    "Grounded CitationAccuracy（引用校验准确率）",
                    grounding.getCitationVerification().getCitationAccuracy(),
                    true
            ));
        }
        if (grounding.getUnsupportedClaimReport() != null) {
            int total = grounding.getUnsupportedClaimReport().getTotalClaims();
            double unsupportedRate = total == 0
                    ? 0.0
                    : (double) grounding.getUnsupportedClaimReport().getUnsupportedClaims() / total;
            metrics.add(HealthMetricValue.of(
                    "UNSUPPORTED_CLAIM_RATE",
                    "UnsupportedClaimRate（未支持主张率）",
                    unsupportedRate,
                    true
            ));
            metrics.add(HealthMetricValue.of(
                    "GROUNDED_FAITHFULNESS",
                    "Grounded Faithfulness（基于未支持主张）",
                    1.0 - unsupportedRate,
                    true
            ));
        }
        if (grounding.getRefusalDecision() != null) {
            metrics.add(HealthMetricValue.of(
                    "GROUNDED_REFUSAL_DECISION",
                    "Grounded RefusalDecision（拒答决策）",
                    grounding.getRefusalDecision().isShouldRefuse() ? 1.0 : 0.0,
                    true
            ));
        }
        if (grounding.getGroundingScore() != null) {
            metrics.add(HealthMetricValue.of(
                    "ANSWER_GROUNDING_SCORE",
                    "AnswerGroundingScore（可信回答评分）",
                    grounding.getGroundingScore().getGroundingScore() / 100.0,
                    true
            ));
        }
        return metrics;
    }

    private RagEvaluationRetrievalStrategy toEvaluationStrategy(RetrievalRoutingDecision decision) {
        if (decision == null || decision.getStrategy() == null) {
            return RagEvaluationRetrievalStrategy.VECTOR_ONLY;
        }
        return switch (decision.getStrategy()) {
            case VECTOR_WITH_METADATA_FILTER -> RagEvaluationRetrievalStrategy.VECTOR_WITH_METADATA_FILTER;
            case HYBRID_RRF -> RagEvaluationRetrievalStrategy.HYBRID_RRF;
            case HYBRID_RRF_RERANK -> RagEvaluationRetrievalStrategy.HYBRID_RRF_RERANK;
            case HYBRID_RRF_RERANK_PARENT_CONTEXT -> RagEvaluationRetrievalStrategy.HYBRID_RRF_RERANK_PARENT_CONTEXT;
        };
    }

    private String buildHeuristicAnswer(List<EvaluationRetrievedChunk> chunks) {
        if (chunks == null || chunks.isEmpty()) {
            return "根据当前检索到的文档内容，无法确定。";
        }
        EvaluationRetrievedChunk top = chunks.get(0);
        return top.getTextSnippet() == null ? "检索到相关内容。" : top.getTextSnippet();
    }

    private Map<String, Object> buildSummary(
            RagEvaluationRunEntity run,
            RagHealthQualityScoreCalculator.RagQualityScoreResult scoreResult,
            RagQualityVetoRuleService.VetoResult vetoResult,
            List<HealthMetricValue> retrieval,
            List<HealthMetricValue> generation
    ) {
        Map<String, Object> summary = new HashMap<>();
        summary.put("overallScore", vetoResult.getFinalScore());
        summary.put("profile", run.getScoringProfile().name());
        summary.put("strategy", run.getStrategy().name());
        summary.put("vetoRulesApplied", vetoResult.getRulesApplied());
        summary.put("metricBreakdown", scoreResult.getBreakdown());
        summary.put("retrievalMetrics", retrieval);
        summary.put("generationMetrics", generation);
        return summary;
    }

    private List<HealthMetricValue> readMetrics(String json) {
        List<HealthMetricValue> metrics = JsonFieldCodec.readObject(json, new TypeReference<List<HealthMetricValue>>() {
        });
        return metrics == null ? List.of() : metrics;
    }

    private List<HealthMetricValue> averageMetrics(List<HealthMetricValue> metrics) {
        Map<String, List<Double>> grouped = metrics.stream()
                .filter(HealthMetricValue::isAvailable)
                .filter(metric -> metric.getRawValue() != null)
                .collect(Collectors.groupingBy(
                        HealthMetricValue::getCode,
                        Collectors.mapping(HealthMetricValue::getRawValue, Collectors.toList())
                ));
        List<HealthMetricValue> averaged = new ArrayList<>();
        for (Map.Entry<String, List<Double>> entry : grouped.entrySet()) {
            double avg = entry.getValue().stream().mapToDouble(Double::doubleValue).average().orElse(0.0);
            HealthMetricValue sample = metrics.stream().filter(m -> entry.getKey().equals(m.getCode())).findFirst().orElse(null);
            averaged.add(HealthMetricValue.of(
                    entry.getKey(),
                    sample == null ? entry.getKey() : sample.getDisplayName(),
                    avg,
                    sample != null && sample.isHeuristic()
            ));
        }
        return averaged;
    }

    @SuppressWarnings("unchecked")
    private Map<String, Double> metricMap(Map<String, Object> summary) {
        Map<String, Double> result = new HashMap<>();
        Object retrieval = summary.get("retrievalMetrics");
        if (retrieval instanceof List<?> list) {
            for (Object item : list) {
                if (item instanceof Map<?, ?> map) {
                    Object code = map.get("code");
                    Object raw = map.get("rawValue");
                    if (code != null && raw instanceof Number number) {
                        result.put(String.valueOf(code), number.doubleValue());
                    }
                } else if (item instanceof HealthMetricValue metric && metric.isAvailable() && metric.getRawValue() != null) {
                    result.put(metric.getCode(), metric.getRawValue());
                }
            }
        }
        Object generation = summary.get("generationMetrics");
        if (generation instanceof List<?> list) {
            for (Object item : list) {
                if (item instanceof Map<?, ?> map) {
                    Object code = map.get("code");
                    Object raw = map.get("rawValue");
                    if (code != null && raw instanceof Number number) {
                        result.put(String.valueOf(code), number.doubleValue());
                    }
                } else if (item instanceof HealthMetricValue metric && metric.isAvailable() && metric.getRawValue() != null) {
                    result.put(metric.getCode(), metric.getRawValue());
                }
            }
        }
        return result;
    }

    private RagEvaluationRunEntity findRunOrThrow(Long runId) {
        return runRepository.findById(runId)
                .orElseThrow(() -> BusinessException.invalidRequest("run not found: " + runId));
    }

    private RagEvaluationRunResponse toRunResponse(RagEvaluationRunEntity entity) {
        Map<String, Object> summary = JsonFieldCodec.readMap(entity.getSummaryJson());
        Map<String, Object> diagnosis = JsonFieldCodec.readObject(
                entity.getDiagnosisJson(),
                new TypeReference<Map<String, Object>>() {
                }
        );
        if (diagnosis == null) {
            diagnosis = Map.of();
        }
        return new RagEvaluationRunResponse(
                entity.getId(),
                entity.getDatasetId(),
                entity.getName(),
                entity.getStatus(),
                entity.getStrategy(),
                RagHealthDisplayTexts.strategy(entity.getStrategy()),
                entity.getScoringProfile(),
                RagHealthDisplayTexts.scoringProfile(entity.getScoringProfile()),
                entity.getTopK(),
                entity.getRetrievalTopK(),
                entity.getRerankTopN(),
                entity.getCollectionId(),
                JsonFieldCodec.readMap(entity.getMetadataFilterJson()),
                entity.getExecuteGeneration(),
                entity.getStartedAt(),
                entity.getCompletedAt(),
                entity.getTotalCases(),
                entity.getCompletedCases(),
                entity.getFailedCases(),
                entity.getOverallScore(),
                RagHealthDisplayTexts.scoreLevel(entity.getOverallScore() == null ? -1 : entity.getOverallScore()),
                summary,
                diagnosis
        );
    }

    private RagEvaluationCaseResultResponse toCaseResultResponse(RagEvaluationCaseResultEntity entity) {
        List<EvaluationRetrievedChunk> chunks = JsonFieldCodec.readObject(
                entity.getRetrievedChunksJson(),
                new TypeReference<List<EvaluationRetrievedChunk>>() {
                }
        );
        if (chunks == null) {
            chunks = List.of();
        }
        List<String> citations = JsonFieldCodec.readStringList(entity.getCitationsJson());
        List<HealthMetricValue> retrievalMetrics = readMetrics(entity.getRetrievalMetricsJson());
        List<HealthMetricValue> generationMetrics = readMetrics(entity.getGenerationMetricsJson());
        Map<String, Object> diagnosis = JsonFieldCodec.readObject(
                entity.getDiagnosisJson(),
                new TypeReference<Map<String, Object>>() {
                }
        );
        if (diagnosis == null) {
            diagnosis = Map.of();
        }
        return new RagEvaluationCaseResultResponse(
                entity.getId(),
                entity.getRunId(),
                entity.getCaseId(),
                entity.getQuery(),
                entity.getStrategy(),
                entity.getTopK(),
                chunks,
                entity.getGeneratedAnswer(),
                citations,
                retrievalMetrics,
                generationMetrics,
                entity.getQualityScore(),
                diagnosis,
                entity.getLatencyMs(),
                entity.getErrorCode(),
                entity.getErrorMessage()
        );
    }
}
