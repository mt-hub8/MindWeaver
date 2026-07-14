package com.tuoman.ai_task_orchestrator.grounding;

import org.springframework.stereotype.Service;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.function.Function;
import java.util.stream.Collectors;

/**
 * 未支持主张检测器。
 *
 * 该类扫描回答中的关键句，识别缺少 citation、引用不存在、或精确实体不在 cited chunk 中的 claim。
 * 它补充 CitationVerificationService：前者看 claim 是否被引用，后者看引用是否来自 final context 并支持 claim。
 */
@Service
public class UnsupportedClaimDetector {

    public UnsupportedClaimReport detect(String answer, GroundedContextBundle bundle) {
        // 无 context 时出现确定性 claim 属于 hallucination risk。
        // 这种情况下拒答不是失败，而是 grounded answer contract 的正确执行。
        List<String> sentences = GroundingTextUtils.sentences(answer);
        Map<String, GroundedContextChunk> contextByKey = bundle == null || bundle.getChunks() == null
                ? Map.of()
                : bundle.getChunks().stream().collect(Collectors.toMap(GroundedContextChunk::getCitationKey, Function.identity()));
        List<ClaimDetail> details = new ArrayList<>();
        int supported = 0;
        int unsupported = 0;
        int missing = 0;
        boolean hallucinationRisk = false;

        for (int i = 0; i < sentences.size(); i++) {
            String sentence = sentences.get(i);
            if (!GroundingTextUtils.isKeyClaim(sentence)) {
                continue;
            }
            List<String> keys = GroundingTextUtils.citationKeys(sentence);
            ClaimIssueType issue = ClaimIssueType.NONE;
            String reason = "claim has at least one valid supporting citation";
            if (keys.isEmpty()) {
                issue = contextByKey.isEmpty() ? ClaimIssueType.HALLUCINATION_RISK : ClaimIssueType.MISSING_CITATION;
                reason = contextByKey.isEmpty()
                        ? "no context exists but answer contains a deterministic claim"
                        : "key claim has no citation";
            } else if (keys.stream().anyMatch(key -> !contextByKey.containsKey(key))) {
                issue = ClaimIssueType.INVALID_CITATION;
                reason = "claim cites a key that is not in final context";
            } else {
                // 精确实体检查优先关注版本号、数字、配置项、API path、类名等高风险 token。
                // 这些内容一旦编造，用户很容易把错误答案当成可执行事实。
                String citedText = keys.stream()
                        .map(contextByKey::get)
                        .map(GroundedContextChunk::getText)
                        .collect(Collectors.joining("\n"));
                issue = exactEntityIssue(sentence, citedText);
                if (issue != ClaimIssueType.NONE) {
                    reason = "claim contains an exact entity that does not appear in cited chunk";
                }
            }
            if (issue == ClaimIssueType.NONE) {
                supported++;
            } else {
                unsupported++;
                if (issue == ClaimIssueType.MISSING_CITATION) {
                    missing++;
                }
                if (issue == ClaimIssueType.HALLUCINATION_RISK) {
                    hallucinationRisk = true;
                }
            }
            details.add(ClaimDetail.builder()
                    .claimText(sentence)
                    .sentenceIndex(i)
                    .citationKeys(keys)
                    .issueType(issue)
                    .severity(issue == ClaimIssueType.NONE ? "LOW" : "HIGH")
                    .reason(reason)
                    .build());
        }
        return UnsupportedClaimReport.builder()
                .totalClaims(details.size())
                .supportedClaims(supported)
                .unsupportedClaims(unsupported)
                .missingCitationClaims(missing)
                .hallucinationRisk(hallucinationRisk)
                .claimDetails(details)
                .build();
    }

    private ClaimIssueType exactEntityIssue(String claim, String citedText) {
        if (!GroundingTextUtils.containsAnyExact(citedText, GroundingTextUtils.symbols(claim))
                || !GroundingTextUtils.containsAnyExact(citedText, GroundingTextUtils.configKeys(claim))
                || !GroundingTextUtils.containsAnyExact(citedText, GroundingTextUtils.apiPaths(claim))) {
            return ClaimIssueType.UNSUPPORTED_SYMBOL;
        }
        if (!GroundingTextUtils.containsAnyExact(citedText, GroundingTextUtils.versions(claim))) {
            return ClaimIssueType.UNSUPPORTED_VERSION;
        }
        if (!GroundingTextUtils.containsAnyExact(citedText, GroundingTextUtils.numbers(claim))) {
            return ClaimIssueType.UNSUPPORTED_NUMBER;
        }
        return ClaimIssueType.NONE;
    }
}
