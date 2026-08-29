package com.tuoman.ai_task_orchestrator.prompt;

import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;

import java.util.Collections;
import java.util.Map;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

@Slf4j
@Component
/**
 * V1.1 Prompt Template 渲染器。
 *
 * 将 Task 输入、taskId 和模型名注入模板，避免执行层硬编码 prompt。
 * 模板变量缺失时显式失败，比静默生成残缺 prompt 更容易排障。
 */
public class PromptTemplateRenderer {

    private static final Pattern VARIABLE_PATTERN = Pattern.compile("\\{\\{\\s*([a-zA-Z0-9_]+)\\s*\\}\\}");

    public String render(String templateContent, Map<String, String> variables) {
        if (templateContent == null || templateContent.isBlank()) {
            throw new IllegalArgumentException("Template content must not be blank");
        }

        Map<String, String> safeVariables = variables == null ? Collections.emptyMap() : variables;
        log.debug("Start rendering prompt template");

        Matcher matcher = VARIABLE_PATTERN.matcher(templateContent);
        StringBuffer renderedContent = new StringBuffer();

        while (matcher.find()) {
            String variableName = matcher.group(1);
            if (!safeVariables.containsKey(variableName)) {
                log.warn("Missing template variable: {}", variableName);
                throw new IllegalArgumentException("Missing template variable: " + variableName);
            }

            String variableValue = safeVariables.get(variableName);
            matcher.appendReplacement(
                    renderedContent,
                    Matcher.quoteReplacement(variableValue == null ? "" : variableValue)
            );
        }

        matcher.appendTail(renderedContent);
        log.debug("Prompt template rendered successfully");

        return renderedContent.toString();
    }
}
