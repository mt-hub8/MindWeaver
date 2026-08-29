package com.tuoman.ai_task_orchestrator.retrieval;

import java.util.List;

/**
 * keyword retrieval 抽象。
 *
 * dense retrieval 依赖语义向量，keyword retrieval 负责补足类名、配置项、API path、
 * 命令和版本号等字面匹配场景。实现必须接收 RetrievalFilter，不能返回超出 collection、
 * version、status 或生命周期范围的候选。
 */
public interface KeywordRetriever {

    KeywordRetrievalResponse search(String query, RetrievalFilter filter, int topK);

    String name();

    record KeywordCandidate(
            int rank,
            Long documentId,
            String documentTitle,
            Long chunkId,
            String sectionPath,
            String content,
            double score,
            String matchReason
    ) {
    }

    record KeywordRetrievalResponse(List<KeywordCandidate> candidates, long latencyMs) {
    }
}
