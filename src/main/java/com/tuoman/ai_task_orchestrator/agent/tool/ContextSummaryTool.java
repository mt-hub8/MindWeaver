package com.tuoman.ai_task_orchestrator.agent.tool;

import com.tuoman.ai_task_orchestrator.common.error.ErrorCode;
import com.tuoman.ai_task_orchestrator.llm.LlmGenerateOptions;
import com.tuoman.ai_task_orchestrator.llm.LlmGenerateResult;
import com.tuoman.ai_task_orchestrator.llm.LlmProvider;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

@Component
@RequiredArgsConstructor
public class ContextSummaryTool implements AgentTool {

    private final LlmProvider llmProvider;

    @Override
    public String toolName() {
        return AgentToolNames.CONTEXT_SUMMARY;
    }

    @Override
    public String displayName() {
        return "总结检索结果";
    }

    @Override
    public String description() {
        return "对知识库检索得到的片段进行结构化摘要，提炼关键要点与局限。";
    }

    @Override
    public Map<String, Object> inputSchema() {
        return Map.of(
                "matchedChunks", Map.of("type", "array", "required", true, "description", "检索片段列表"),
                "taskObjective", Map.of("type", "string", "required", true, "description", "任务目标")
        );
    }

    @Override
    public Map<String, Object> outputSchema() {
        return Map.of(
                "summary", "摘要文本",
                "keyPoints", "关键要点列表",
                "limitations", "局限说明",
                "llmMetadata", "模型调用元数据，可选"
        );
    }

    @Override
    public boolean enabled() {
        return true;
    }

    @Override
    @SuppressWarnings("unchecked")
    public ToolExecutionResult execute(Map<String, Object> input, ToolExecutionContext context) {
        long started = System.nanoTime();
        if (input == null) {
            return ToolExecutionResult.failure(
                    ErrorCode.AGENT_TOOL_INPUT_INVALID.name(),
                    "工具输入不能为空",
                    elapsed(started)
            );
        }
        Object noContext = input.get("noContext");
        List<Map<String, Object>> matchedChunks = input.get("matchedChunks") instanceof List<?> list
                ? (List<Map<String, Object>>) list
                : List.of();
        String taskObjective = input.get("taskObjective") == null
                ? context.taskObjective()
                : String.valueOf(input.get("taskObjective"));

        if (Boolean.TRUE.equals(noContext) || matchedChunks.isEmpty()) {
            return ToolExecutionResult.success(noSummaryOutput("无可总结内容"), elapsed(started));
        }

        String systemPrompt = """
                你是一个企业知识库任务执行助手。请基于提供的检索片段，用中文输出简洁的结构化摘要。
                不要编造片段中不存在的信息。
                """;
        String userPrompt = buildUserPrompt(taskObjective, matchedChunks);

        LlmGenerateOptions options = new LlmGenerateOptions();
        options.setTaskId(context.taskId());
        LlmGenerateResult llmResult = llmProvider.generate(systemPrompt, userPrompt, options);
        if (!llmResult.isSuccess() || llmResult.getContent() == null || llmResult.getContent().isBlank()) {
            return ToolExecutionResult.failure(
                    ErrorCode.AGENT_TOOL_EXECUTION_FAILED.name(),
                    llmResult.getErrorMessage() == null ? "总结检索结果失败" : llmResult.getErrorMessage(),
                    elapsed(started)
            );
        }

        Map<String, Object> output = new LinkedHashMap<>();
        output.put("summary", llmResult.getContent());
        output.put("keyPoints", extractKeyPoints(llmResult.getContent()));
        output.put("limitations", "摘要基于当前检索片段，可能未覆盖全部知识库内容。");
        output.put("llmMetadata", llmMetadata(llmResult));
        return ToolExecutionResult.success(output, elapsed(started));
    }

    private Map<String, Object> noSummaryOutput(String message) {
        Map<String, Object> output = new LinkedHashMap<>();
        output.put("summary", message);
        output.put("keyPoints", List.of());
        output.put("limitations", "当前没有可用于总结的检索片段。");
        return output;
    }

    private String buildUserPrompt(String taskObjective, List<Map<String, Object>> matchedChunks) {
        StringBuilder prompt = new StringBuilder();
        prompt.append("任务目标：\n").append(taskObjective).append("\n\n");
        prompt.append("检索片段：\n");
        for (Map<String, Object> chunk : matchedChunks) {
            prompt.append("- [").append(chunk.get("sourceIndex")).append("] ");
            prompt.append(chunk.getOrDefault("snippet", chunk.getOrDefault("contentSnippet", ""))).append("\n");
        }
        prompt.append("\n请输出：\n1. 一段摘要\n2. 3-5 条关键要点（使用 - 列表）\n");
        return prompt.toString();
    }

    private List<String> extractKeyPoints(String content) {
        List<String> points = new ArrayList<>();
        for (String line : content.split("\n")) {
            String trimmed = line.trim();
            if (trimmed.startsWith("-") || trimmed.startsWith("•")) {
                points.add(trimmed.replaceFirst("^[-•]\\s*", ""));
            }
        }
        if (points.isEmpty() && content != null && !content.isBlank()) {
            points.add(content.length() > 120 ? content.substring(0, 120) + "..." : content);
        }
        return points;
    }

    private Map<String, Object> llmMetadata(LlmGenerateResult result) {
        Map<String, Object> metadata = new LinkedHashMap<>();
        metadata.put("provider", result.getProvider());
        metadata.put("model", result.getModel());
        metadata.put("inputTokens", result.getInputTokens());
        metadata.put("outputTokens", result.getOutputTokens());
        metadata.put("latencyMs", result.getLatencyMs());
        metadata.put("finishReason", result.getFinishReason());
        return metadata;
    }

    private long elapsed(long started) {
        return (System.nanoTime() - started) / 1_000_000;
    }
}
