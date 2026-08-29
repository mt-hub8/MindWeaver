package com.tuoman.ai_task_orchestrator.grounding;

import com.tuoman.ai_task_orchestrator.queryunderstanding.QueryUnderstandingResult;
import org.springframework.stereotype.Component;

import java.util.List;

/**
 * V18 Grounded Answer prompt 构造器。
 *
 * 它把 GroundedAnswerContract、QueryUnderstandingResult 和 GroundedContextBundle 组合成 LLM prompt。
 * 这里的目标不是让模型“自由回答”，而是约束模型只基于 final context 输出可校验回答。
 */
@Component
public class GroundedAnswerPromptBuilder {

    private static final int PROMPT_BUDGET = 9000;

    public String buildPrompt(
            String query,
            GroundedContextBundle bundle,
            GroundedAnswerContract contract,
            QueryUnderstandingResult understanding
    ) {
        // 无 context 时也构造拒答 prompt，避免 LLM 使用外部知识补答案。
        // no context / low confidence 的拒答是可信 RAG 的正常业务分支。
        if (bundle == null || bundle.isEmpty()) {
            return buildRefusalPrompt(query, contract);
        }
        GroundedAnswerContract safeContract = contract == null
                ? new GroundedAnswerContract(AnswerContractMode.defaultMode())
                : contract;
        StringBuilder prompt = new StringBuilder();
        prompt.append("系统角色：你是基于资料回答的知识库助手。\n\n");
        prompt.append("回答规则：\n");
        prompt.append("- 只能使用【资料】中的内容；\n");
        prompt.append("- 每个关键结论后必须给出引用，例如 [1]；\n");
        prompt.append("- 如果资料不足，回答“根据当前资料无法确认”；\n");
        prompt.append("- 不要编造文档、章节、配置项、接口路径、版本或文件路径；\n");
        prompt.append("- 不要把推断说成事实；如果需要推断，必须写“根据当前资料推断”；\n");
        prompt.append("- 如果资料冲突，要指出“资料之间存在冲突”。\n");
        if (safeContract.getMode() == AnswerContractMode.STRICT) {
            prompt.append("- STRICT 模式：所有关键结论、版本、接口、配置项、数字都必须带 citation。\n");
        } else if (safeContract.getMode() == AnswerContractMode.EXPLORATORY) {
            prompt.append("- EXPLORATORY 模式：可以做轻度总结和推断，但必须标注推断且不能脱离资料。\n");
        }
        prompt.append("\n用户问题：\n").append(query == null ? "" : query).append("\n\n");
        appendUnderstanding(prompt, understanding);
        prompt.append("【资料】\n");
        // prompt 中的 citationKey 与 bundle 中的 chunk 一一对应。
        // 模型不能引用不在这里出现的 citation，否则后续校验会失败。
        for (GroundedContextChunk chunk : bundle.getChunks()) {
            prompt.append(chunk.getCitationKey()).append("\n");
            prompt.append("文档：").append(value(chunk.getDocumentTitle())).append("\n");
            prompt.append("章节：").append(value(chunk.getSectionPath())).append("\n");
            prompt.append("版本：").append(value(chunk.getVersion())).append("\n");
            prompt.append("类型：").append(value(chunk.getDocType())).append("\n");
            prompt.append("chunkId：").append(value(chunk.getChunkId())).append("\n");
            prompt.append("内容：\n").append(value(chunk.getText())).append("\n\n");
            if (prompt.length() > PROMPT_BUDGET) {
                prompt.append("【资料已按预算截断】\n");
                break;
            }
        }
        prompt.append("输出格式：\n");
        prompt.append("直接回答：\n...\n\n");
        prompt.append("依据：\n- [1] ...\n\n");
        prompt.append("不确定或缺失：\n- ...\n");
        if (prompt.length() > PROMPT_BUDGET) {
            return prompt.substring(0, PROMPT_BUDGET);
        }
        return prompt.toString();
    }

    public String buildRefusalPrompt(String query, GroundedAnswerContract contract) {
        return "系统角色：你是基于资料回答的知识库助手。\n\n"
                + "当前没有可用【资料】。不得使用外部知识回答。\n"
                + "用户问题：\n" + (query == null ? "" : query) + "\n\n"
                + "请直接回答：根据当前资料无法确认。";
    }

    private void appendUnderstanding(StringBuilder prompt, QueryUnderstandingResult understanding) {
        if (understanding == null) {
            return;
        }
        prompt.append("查询理解：\n");
        prompt.append("- queryType：").append(understanding.getQueryType()).append("\n");
        prompt.append("- versionHint：").append(value(understanding.getVersionHint())).append("\n");
        prompt.append("- docTypeHint：").append(value(understanding.getDocTypeHint())).append("\n");
        appendList(prompt, "codeSymbols", understanding.getCodeSymbols());
        appendList(prompt, "configKeys", understanding.getConfigKeys());
        appendList(prompt, "apiPaths", understanding.getApiPaths());
        prompt.append("\n");
    }

    private void appendList(StringBuilder prompt, String label, List<String> values) {
        if (values != null && !values.isEmpty()) {
            prompt.append("- ").append(label).append("：").append(String.join(", ", values)).append("\n");
        }
    }

    private String value(Object value) {
        return value == null ? "-" : String.valueOf(value);
    }
}
