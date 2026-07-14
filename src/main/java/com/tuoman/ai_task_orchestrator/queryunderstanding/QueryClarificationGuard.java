package com.tuoman.ai_task_orchestrator.queryunderstanding;

import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;

import java.util.ArrayList;
import java.util.List;

/**
 * 模糊问题澄清保护器。
 *
 * 它在 retrieval 前做最后一道 guard：当 query 低置信度、缺少实体或全库规模过大时，
 * 返回 clarificationRequired，而不是继续盲目召回。
 *
 * 关键不变量：用户已经手动选择 collection 时不强制澄清，因为范围已经被人为收窄。
 */
@Service
@RequiredArgsConstructor
public class QueryClarificationGuard {

    private static final String DEFAULT_QUESTION =
            "当前问题比较模糊，建议先选择知识库分组，避免跨项目文档混入。";

    private final QueryUnderstandingProperties properties;

    public GuardResult evaluate(
            QueryUnderstandingResult understanding,
            RetrievalRoutingDecision decision,
            UserSelectedFilters userSelectedFilters,
            long documentCount,
            long collectionCount
    ) {
        List<String> reasons = new ArrayList<>();
        // 手动 collection scope 是用户给出的强约束，优先于自动澄清策略。
        boolean selectedCollection = userSelectedFilters != null && userSelectedFilters.hasCollection();
        if (!properties.isClarificationEnabled() || selectedCollection) {
            return new GuardResult(false, null, reasons);
        }
        if (understanding.getQueryType() == QueryType.AMBIGUOUS) {
            reasons.add("queryType=AMBIGUOUS");
        }
        if (understanding.getConfidence() < properties.getMinConfidence()) {
            reasons.add("confidence_below_threshold");
        }
        if (!understanding.hasEntities()) {
            reasons.add("no_detected_entity");
        }
        if (documentCount > properties.getMaxGlobalSearchDocuments()) {
            reasons.add("document_count_exceeds_threshold");
        }
        if (collectionCount > properties.getMaxGlobalSearchCollections()) {
            reasons.add("collection_count_exceeds_threshold");
        }
        boolean required = !reasons.isEmpty()
                && (understanding.getQueryType() == QueryType.AMBIGUOUS
                || understanding.getConfidence() < properties.getMinConfidence()
                || documentCount > properties.getMaxGlobalSearchDocuments()
                || collectionCount > properties.getMaxGlobalSearchCollections());
        String question = decision != null && decision.getClarificationQuestion() != null
                ? decision.getClarificationQuestion()
                : DEFAULT_QUESTION;
        return new GuardResult(required, required ? question : null, reasons);
    }

    public record GuardResult(boolean clarificationRequired, String clarificationQuestion, List<String> reasons) {
    }
}
