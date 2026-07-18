package com.tuoman.ai_task_orchestrator.agent;

import com.tuoman.ai_task_orchestrator.dto.AgentTaskCitationResponse;
import com.tuoman.ai_task_orchestrator.memory.MemoryContextBundle;
import org.springframework.stereotype.Component;

import java.util.List;
import java.util.Map;

@Component
public class AgentTaskPromptBuilder {

    private static final int SNIPPET_MAX = 2000;

    public String buildFinalReportSystemPrompt() {
        return """
                你是一个企业知识库任务执行助手。你需要基于工具检索和总结得到的结果完成任务。
                不要使用工具结果之外的信息编造内容。如果信息不足，需要明确说明。
                输出必须使用中文，并按指定结构组织答案。
                """;
    }

    public String buildFinalReportSystemPrompt(String profileInstruction) {
        if (profileInstruction == null || profileInstruction.isBlank()) {
            return buildFinalReportSystemPrompt();
        }
        return buildFinalReportSystemPrompt()
                + "\n\n【Agent Profile】\n"
                + profileInstruction.trim();
    }

    @SuppressWarnings("unchecked")
    public String buildFinalReportUserPrompt(
            String objective,
            String scopeLabel,
            Map<String, Object> knowledgeSearchOutput,
            Map<String, Object> contextSummaryOutput,
            List<AgentTaskCitationResponse> citations
    ) {
        StringBuilder prompt = new StringBuilder();
        prompt.append("任务目标：\n").append(objective).append("\n\n");
        prompt.append("知识库范围：\n").append(scopeLabel).append("\n\n");

        prompt.append("知识库检索结果摘要：\n");
        if (knowledgeSearchOutput != null && Boolean.TRUE.equals(knowledgeSearchOutput.get("noContext"))) {
            prompt.append("（无可用检索片段）\n\n");
        } else if (citations != null && !citations.isEmpty()) {
            for (AgentTaskCitationResponse citation : citations) {
                prompt.append("[").append(citation.getSourceIndex()).append("] ");
                prompt.append(truncate(citation.getContentSnippet())).append("\n");
            }
            prompt.append("\n");
        }

        prompt.append("上下文总结结果：\n");
        if (contextSummaryOutput != null) {
            prompt.append("摘要：").append(contextSummaryOutput.getOrDefault("summary", "")).append("\n");
            Object keyPoints = contextSummaryOutput.get("keyPoints");
            if (keyPoints instanceof List<?> list && !list.isEmpty()) {
                prompt.append("关键要点：\n");
                for (Object point : list) {
                    prompt.append("- ").append(point).append("\n");
                }
            }
            prompt.append("局限：").append(contextSummaryOutput.getOrDefault("limitations", "")).append("\n\n");
        }

        prompt.append("""
                请按以下结构输出最终报告：
                ## 任务结论
                ## 关键依据
                ## 风险 / 不确定性
                ## 下一步建议
                ## 引用来源
                在正文中使用 [1]、[2] 等引用标记对应上下文编号。
                """);
        return prompt.toString();
    }

    public String buildFinalReportUserPrompt(
            String objective,
            String scopeLabel,
            Map<String, Object> knowledgeSearchOutput,
            Map<String, Object> contextSummaryOutput,
            List<AgentTaskCitationResponse> citations,
            String memoryPromptSection,
            MemoryContextBundle memoryContext
    ) {
        String base = buildFinalReportUserPrompt(
                objective,
                scopeLabel,
                knowledgeSearchOutput,
                contextSummaryOutput,
                citations
        );
        String memory = memoryPromptSection == null || memoryPromptSection.isBlank()
                ? "【长期记忆】\n（本次没有可用记忆）\n"
                : memoryPromptSection;
        return memory + "\n【知识库资料】\n" + base;
    }

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
