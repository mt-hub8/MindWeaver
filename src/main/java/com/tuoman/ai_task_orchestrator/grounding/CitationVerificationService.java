package com.tuoman.ai_task_orchestrator.grounding;

import com.tuoman.ai_task_orchestrator.queryunderstanding.QueryUnderstandingResult;
import com.tuoman.ai_task_orchestrator.retrieval.RetrievalFilter;
import org.springframework.stereotype.Service;

import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Set;
import java.util.function.Function;
import java.util.stream.Collectors;

/**
 * V18 Citation Verification 服务。
 *
 * 检索正确不等于回答可信：LLM 仍可能错引、漏引或把 context 外的信息写进答案。
 * 该服务在生成后检查 citation key 是否来自 final context，并用启发式方法判断引用是否支持对应 claim。
 *
 * 关键约束：citation 必须来自 final context；启发式 verification 不能宣称等价于人工事实核验。
 */
@Service
public class CitationVerificationService {

    private final UnsupportedClaimDetector unsupportedClaimDetector;

    public CitationVerificationService(UnsupportedClaimDetector unsupportedClaimDetector) {
        this.unsupportedClaimDetector = unsupportedClaimDetector;
    }

    public CitationVerificationResult verify(
            String answer,
            GroundedContextBundle bundle,
            RetrievalFilter filter,
            QueryUnderstandingResult understanding
    ) {
        // 只用 final context 建立 citation 索引。
        // 检索候选但未进入 bundle 的 chunk 不能被答案引用。
        Map<String, GroundedContextChunk> contextByKey = bundle == null || bundle.getChunks() == null
                ? Map.of()
                : bundle.getChunks().stream().collect(Collectors.toMap(GroundedContextChunk::getCitationKey, Function.identity()));
        UnsupportedClaimReport claimReport = unsupportedClaimDetector.detect(answer, bundle);
        List<String> answerKeys = GroundingTextUtils.citationKeys(answer);
        Set<String> uniqueKeys = new HashSet<>(answerKeys);
        List<Citation> details = new ArrayList<>();
        List<String> warnings = new ArrayList<>();
        int verified = 0;
        int weak = 0;
        int unsupported = 0;
        int invalid = 0;

        // EXACT/PARTIAL/WEAK/UNSUPPORTED 表示启发式支持强度。
        // UNKNOWN 用于无法判断的场景，不应被当作事实正确。
        for (String key : uniqueKeys) {
            GroundedContextChunk context = contextByKey.get(key);
            if (context == null) {
                invalid++;
                unsupported++;
                details.add(Citation.builder()
                        .citationId(key)
                        .citationKey(key)
                        .supportsClaim(false)
                        .supportLevel(SupportLevel.UNSUPPORTED)
                        .verificationStatus(VerificationStatus.FAILED)
                        .warning("citation key does not exist in final context")
                        .build());
                continue;
            }
            SupportLevel level = supportLevel(answer, key, context, filter, understanding);
            VerificationStatus status = level == SupportLevel.UNSUPPORTED ? VerificationStatus.FAILED : VerificationStatus.HEURISTIC;
            if (level == SupportLevel.EXACT || level == SupportLevel.PARTIAL) {
                verified++;
            } else if (level == SupportLevel.WEAK) {
                weak++;
            } else if (level == SupportLevel.UNSUPPORTED) {
                unsupported++;
            }
            details.add(Citation.builder()
                    .citationId(key)
                    .citationKey(key)
                    .chunkId(context.getChunkId())
                    .documentId(context.getDocumentId())
                    .documentTitle(context.getDocumentTitle())
                    .collectionId(context.getCollectionId())
                    .version(context.getVersion())
                    .sectionPath(context.getSectionPath())
                    .quoteSnippet(snippet(context.getText(), 300))
                    .supportsClaim(level == SupportLevel.EXACT || level == SupportLevel.PARTIAL)
                    .supportLevel(level)
                    .verificationStatus(status)
                    .warning(level == SupportLevel.UNSUPPORTED ? "citation does not support cited claim heuristically" : null)
                    .build());
        }
        if (claimReport.getMissingCitationClaims() > 0) {
            warnings.add("missing citation on key claims: " + claimReport.getMissingCitationClaims());
        }
        double accuracy = uniqueKeys.isEmpty() ? 0.0 : (double) verified / uniqueKeys.size();
        return CitationVerificationResult.builder()
                .valid(invalid == 0 && unsupported == 0 && claimReport.getMissingCitationClaims() == 0)
                .citationAccuracy(accuracy)
                .totalCitations(uniqueKeys.size())
                .verifiedCitations(verified)
                .weakCitations(weak)
                .unsupportedCitations(unsupported)
                .missingCitationCount(claimReport.getMissingCitationClaims())
                .invalidCitationCount(invalid)
                .heuristic(true)
                .warnings(warnings)
                .citationDetails(details)
                .build();
    }

    private SupportLevel supportLevel(
            String answer,
            String key,
            GroundedContextChunk context,
            RetrievalFilter filter,
            QueryUnderstandingResult understanding
    ) {
        // scope 校验先于文本重叠。
        // collection/version filter 被破坏时，即使文本相似，也不能算有效 citation。
        if (!scopeValid(context, filter, understanding)) {
            return SupportLevel.UNSUPPORTED;
        }
        String claimText = claimTextForKey(answer, key);
        String contextText = context.getText() == null ? "" : context.getText();
        if (!strictEntitiesSupported(claimText, contextText, understanding)) {
            return SupportLevel.UNSUPPORTED;
        }
        Set<String> claimKeywords = GroundingTextUtils.keywords(claimText);
        Set<String> contextKeywords = GroundingTextUtils.keywords(contextText);
        long overlap = claimKeywords.stream().filter(contextKeywords::contains).count();
        if (claimKeywords.isEmpty()) {
            return SupportLevel.UNKNOWN;
        }
        // 轻量启发式：引用句关键词在 cited chunk 中的覆盖率越高，支持级别越强。
        // 这能发现明显错引，但不能替代人工事实核验。
        double ratio = (double) overlap / claimKeywords.size();
        if (ratio >= 0.55) {
            return SupportLevel.EXACT;
        }
        if (ratio >= 0.25) {
            return SupportLevel.PARTIAL;
        }
        if (overlap > 0) {
            return SupportLevel.WEAK;
        }
        return SupportLevel.UNSUPPORTED;
    }

    private boolean scopeValid(GroundedContextChunk context, RetrievalFilter filter, QueryUnderstandingResult understanding) {
        if (filter != null) {
            if (filter.getCollectionId() != null && !filter.getCollectionId().equals(context.getCollectionId())) {
                return false;
            }
            if (filter.getVersion() != null && !filter.getVersion().equalsIgnoreCase(String.valueOf(context.getVersion()))) {
                return false;
            }
        }
        if (understanding != null && understanding.getVersionHint() != null && context.getVersion() != null) {
            String expected = understanding.getVersionHint().toLowerCase(Locale.ROOT);
            String actual = context.getVersion().toLowerCase(Locale.ROOT);
            return actual.contains(expected) || expected.contains(actual);
        }
        return true;
    }

    private boolean strictEntitiesSupported(String claimText, String contextText, QueryUnderstandingResult understanding) {
        if (!GroundingTextUtils.containsAnyExact(contextText, GroundingTextUtils.symbols(claimText))
                || !GroundingTextUtils.containsAnyExact(contextText, GroundingTextUtils.configKeys(claimText))
                || !GroundingTextUtils.containsAnyExact(contextText, GroundingTextUtils.apiPaths(claimText))) {
            return false;
        }
        if (understanding == null) {
            return true;
        }
        return GroundingTextUtils.containsAnyExact(contextText, understanding.getCodeSymbols())
                && GroundingTextUtils.containsAnyExact(contextText, understanding.getConfigKeys())
                && GroundingTextUtils.containsAnyExact(contextText, understanding.getApiPaths());
    }

    private String claimTextForKey(String answer, String key) {
        return GroundingTextUtils.sentences(answer).stream()
                .filter(sentence -> GroundingTextUtils.citationKeys(sentence).contains(key))
                .collect(Collectors.joining(" "));
    }

    private String snippet(String text, int max) {
        if (text == null) {
            return "";
        }
        return text.length() <= max ? text : text.substring(0, max);
    }
}
