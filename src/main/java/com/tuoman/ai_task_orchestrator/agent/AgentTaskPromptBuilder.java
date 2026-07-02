package com.tuoman.ai_task_orchestrator.agent;

import com.tuoman.ai_task_orchestrator.dto.AgentTaskCitationResponse;
import org.springframework.stereotype.Component;

import java.util.List;

@Component
public class AgentTaskPromptBuilder {

    private static final int SNIPPET_MAX = 2000;

    public String buildSystemPrompt() {
        return """
                你是一个企业知识库任务执行助手。你只能基于提供的知识库上下文完成任务。
                如果上下文不足，需要明确说明不足，不要编造。
                输出必须使用中文，并按指定结构组织答案。
                """;
    }

    public String buildUserPrompt(
            String objective,
            String scopeLabel,
            List<AgentTaskCitationResponse> citations
    ) {
        StringBuilder prompt = new StringBuilder();
        prompt.append("任务目标：\n").append(objective).append("\n\n");
        prompt.append("知识库范围：\n").append(scopeLabel).append("\n\n");
        prompt.append("知识库上下文：\n");
        if (citations == null || citations.isEmpty()) {
            prompt.append("（无可用上下文）\n\n");
        } else {
            for (AgentTaskCitationResponse citation : citations) {
                prompt.append("[").append(citation.getSourceIndex()).append("]\n");
                prompt.append("documentId: ").append(citation.getDocumentId()).append("\n");
                prompt.append("chunkId: ").append(citation.getChunkId()).append("\n");
                prompt.append("content: ").append(truncate(citation.getContentSnippet())).append("\n\n");
            }
        }
        prompt.append("""
                请按以下结构输出：
                ## 任务结论
                ## 关键依据
                ## 风险 / 不确定性
                ## 下一步建议
                ## 引用来源
                在正文中使用 [1]、[2] 等引用标记对应上下文编号。
                """);
        return prompt.toString();
    }

    private String truncate(String content) {
        if (content == null) {
            return "";
        }
        if (content.length() <= SNIPPET_MAX) {
            return content;
        }
        return content.substring(0, SNIPPET_MAX);
    }
}
