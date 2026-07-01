package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.DocumentSearchResultResponse;
import org.springframework.stereotype.Component;

import java.util.List;

@Component
public class RagPromptBuilder {

    public String buildPrompt(String query, List<DocumentSearchResultResponse> chunks) {
        StringBuilder prompt = new StringBuilder();
        prompt.append("你是一个基于给定资料回答问题的助手。\n\n");
        prompt.append("要求：\n");
        prompt.append("1. 只能根据 Context 中的信息回答；\n");
        prompt.append("2. 如果 Context 中没有答案，请回答“无法从当前资料中确定”；\n");
        prompt.append("3. 回答要简洁、准确；\n");
        prompt.append("4. 引用资料时使用 [1]、[2] 这样的编号。\n\n");
        prompt.append("Question:\n");
        prompt.append(query).append("\n\n");
        prompt.append("Context:\n");

        if (chunks == null || chunks.isEmpty()) {
            prompt.append("无可用资料。\n\n");
        } else {
            for (int i = 0; i < chunks.size(); i++) {
                DocumentSearchResultResponse chunk = chunks.get(i);
                prompt.append("[").append(i + 1).append("]\n");
                prompt.append("documentId: ").append(chunk.getDocumentId()).append("\n");
                prompt.append("chunkId: ").append(chunk.getChunkId()).append("\n");
                prompt.append("headingPath: ").append(blankIfNull(chunk.getHeadingPath())).append("\n");
                prompt.append("content:\n");
                prompt.append(blankIfNull(chunk.getContent())).append("\n\n");
            }
        }

        prompt.append("Answer:");
        return prompt.toString();
    }

    private String blankIfNull(String value) {
        return value == null ? "" : value;
    }
}
