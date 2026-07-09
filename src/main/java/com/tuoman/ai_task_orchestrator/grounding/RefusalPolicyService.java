package com.tuoman.ai_task_orchestrator.grounding;

import com.tuoman.ai_task_orchestrator.queryunderstanding.QueryType;
import com.tuoman.ai_task_orchestrator.queryunderstanding.QueryUnderstandingResult;
import com.tuoman.ai_task_orchestrator.queryunderstanding.RetrievalRoutingDecision;
import org.springframework.stereotype.Service;

import java.util.List;

@Service
public class RefusalPolicyService {

    private static final String DEFAULT_REFUSAL = "根据当前知识库资料，无法确认这个问题的答案。建议选择更具体的知识库分组、版本，或补充相关文档后重试。";

    public RefusalDecision decideBeforeGeneration(
            GroundedContextBundle bundle,
            QueryUnderstandingResult understanding,
            RetrievalRoutingDecision routingDecision,
            AnswerContractMode mode
    ) {
        if (routingDecision != null && routingDecision.isClarificationRequired()) {
            return refuse(RefusalReasonCode.QUERY_AMBIGUOUS, "当前问题比较模糊，建议补充检索范围。");
        }
        if (understanding != null && understanding.getQueryType() == QueryType.AMBIGUOUS) {
            return refuse(RefusalReasonCode.QUERY_AMBIGUOUS, "当前问题比较模糊，建议补充知识库分组、版本或文档类型。");
        }
        if (bundle == null || bundle.isEmpty()) {
            if (understanding != null && understanding.getVersionHint() != null) {
                return refuse(RefusalReasonCode.VERSION_NOT_FOUND, "未找到匹配该版本的可引用资料。");
            }
            return refuse(RefusalReasonCode.NO_CONTEXT, "final context 为空。");
        }
        double maxScore = bundle.getChunks().stream()
                .map(GroundedContextChunk::getScore)
                .filter(score -> score != null)
                .mapToDouble(Double::doubleValue)
                .max()
                .orElse(1.0);
        if (maxScore > 0 && maxScore < 0.05) {
            return refuse(RefusalReasonCode.LOW_RETRIEVAL_CONFIDENCE, "检索分数过低。");
        }
        if (mode == AnswerContractMode.STRICT && bundle.getCitations().isEmpty()) {
            return refuse(RefusalReasonCode.STRICT_MODE_NO_EVIDENCE, "STRICT 模式下没有可引用证据。");
        }
        if (understanding != null && understanding.getQueryType() == QueryType.NO_ANSWER_RISK) {
            return RefusalDecision.builder()
                    .shouldRefuse(false)
                    .reasonCode(RefusalReasonCode.NONE)
                    .reasonText("no-answer risk detected; answer must stay conservative")
                    .suggestedNextActions(List.of())
                    .build();
        }
        return RefusalDecision.allow();
    }

    public RefusalDecision decideAfterVerification(CitationVerificationResult verification, AnswerContractMode mode) {
        if (verification == null) {
            return RefusalDecision.allow();
        }
        if (verification.getTotalCitations() > 0 && verification.getVerifiedCitations() == 0
                && verification.getUnsupportedCitations() >= verification.getTotalCitations()) {
            return refuse(RefusalReasonCode.NO_VALID_CITATION, "答案中的引用均未通过启发式支撑校验。");
        }
        if (mode == AnswerContractMode.STRICT && verification.getVerifiedCitations() == 0) {
            return refuse(RefusalReasonCode.STRICT_MODE_NO_EVIDENCE, "STRICT 模式下未得到有效 citation。");
        }
        return RefusalDecision.allow();
    }

    private RefusalDecision refuse(RefusalReasonCode code, String reason) {
        return RefusalDecision.builder()
                .shouldRefuse(true)
                .reasonCode(code)
                .reasonText(reason)
                .suggestedAnswer(DEFAULT_REFUSAL)
                .suggestedNextActions(List.of("选择更具体的知识库分组", "补充版本或文档类型", "上传相关文档后重试"))
                .build();
    }
}
