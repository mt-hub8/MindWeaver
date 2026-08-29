package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.RagCitationResponse;
import org.springframework.stereotype.Component;

import java.util.List;

/**
 * 旧版 RAG prompt 构造器。
 *
 * V2.3 早期 RAG Answer 通过该类把 citation 列表拼成简单上下文 prompt。
 * V18 Grounded Answer 使用 GroundedAnswerPromptBuilder，具备更完整的 contract 和 citation verification。
 *
 * 关键不变量：即使在 legacy prompt 中，也只能使用传入 citations 的内容，不应引入 context 外证据。
 */
@Component
public class RagPromptBuilder {

    private static final int CONTEXT_CONTENT_MAX_LENGTH = 2000;

    public String buildPrompt(String query, List<RagCitationResponse> citations) {
        if (citations == null || citations.isEmpty()) {
            throw new IllegalArgumentException("citations must not be empty");
        }

        StringBuilder prompt = new StringBuilder();
        prompt.append("你是一个基于文档回答问题的助手。\n\n");
        prompt.append("请只根据下面的上下文回答用户问题。\n");
        prompt.append("如果上下文中没有足够信息，请回答：“根据当前检索到的文档内容，无法确定。”\n");
        prompt.append("回答中尽量使用 [1]、[2] 这样的引用标记指向对应来源。\n\n");
        prompt.append("上下文：\n");

        // citations 是 legacy prompt 的唯一证据来源。
        // citation 未进入这里，就不能被模型要求引用。
        for (RagCitationResponse citation : citations) {
            prompt.append("[").append(citation.getSourceIndex()).append("]\n");
            prompt.append("documentId: ").append(citation.getDocumentId()).append("\n");
            prompt.append("chunkId: ").append(citation.getChunkId()).append("\n");
            prompt.append("content: ").append(truncateContent(citation.getContentSnippet())).append("\n\n");
        }

        prompt.append("用户问题：\n");
        prompt.append(query).append("\n\n");
        prompt.append("请给出简洁、准确、可追溯的回答。");
        return prompt.toString();
    }

    private String truncateContent(String content) {
        if (content == null) {
            return "";
        }
        if (content.length() <= CONTEXT_CONTENT_MAX_LENGTH) {
            return content;
        }
        return content.substring(0, CONTEXT_CONTENT_MAX_LENGTH);
    }
}
