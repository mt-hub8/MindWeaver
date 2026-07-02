package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.agent.AgentTaskEventRecorder;
import com.tuoman.ai_task_orchestrator.agent.AgentTaskPromptBuilder;
import com.tuoman.ai_task_orchestrator.agent.AgentTaskProperties;
import com.tuoman.ai_task_orchestrator.agent.tool.AgentTool;
import com.tuoman.ai_task_orchestrator.agent.tool.AgentToolNames;
import com.tuoman.ai_task_orchestrator.agent.tool.AgentToolRegistry;
import com.tuoman.ai_task_orchestrator.agent.tool.ToolExecutionContext;
import com.tuoman.ai_task_orchestrator.agent.tool.ToolExecutionResult;
import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.common.error.ErrorCode;
import com.tuoman.ai_task_orchestrator.dto.AgentTaskCitationResponse;
import com.tuoman.ai_task_orchestrator.entity.AgentTaskCitationEntity;
import com.tuoman.ai_task_orchestrator.entity.AgentTaskEntity;
import com.tuoman.ai_task_orchestrator.entity.AgentTaskStepEntity;
import com.tuoman.ai_task_orchestrator.enums.AgentTaskStep;
import com.tuoman.ai_task_orchestrator.enums.AgentTaskStepType;
import com.tuoman.ai_task_orchestrator.llm.LlmGenerateOptions;
import com.tuoman.ai_task_orchestrator.llm.LlmGenerateResult;
import com.tuoman.ai_task_orchestrator.llm.LlmProvider;
import com.tuoman.ai_task_orchestrator.repository.AgentTaskCitationRepository;
import com.tuoman.ai_task_orchestrator.repository.AgentTaskRepository;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;

import java.time.LocalDateTime;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

@Slf4j
@Service
@RequiredArgsConstructor
public class AgentTaskWorkflowService {

    private static final String NO_CONTEXT_RESULT = "当前范围内没有可用于执行任务的知识库内容。";

    private final AgentTaskRepository agentTaskRepository;

    private final AgentTaskCitationRepository agentTaskCitationRepository;

    private final AgentTaskStepService agentTaskStepService;

    private final AgentToolRegistry agentToolRegistry;

    private final AgentTaskEventRecorder agentTaskEventRecorder;

    private final AgentTaskPromptBuilder agentTaskPromptBuilder;

    private final LlmProvider llmProvider;

    private final AgentTaskProperties agentTaskProperties;

    public void executeWorkflow(AgentTaskEntity task) {
        Long taskId = task.getId();
        List<AgentTaskStepEntity> steps = agentTaskStepService.listSteps(taskId);
        if (steps.isEmpty()) {
            steps = agentTaskStepService.createFixedPlan(taskId);
            agentTaskEventRecorder.recordStepPlanCreated(taskId);
        }

        ToolExecutionContext context = buildContext(task);
        Map<String, Object> searchOutput = null;
        boolean noContext = false;

        AgentTaskStepEntity searchStep = steps.get(0);
        searchOutput = executeToolStep(task, searchStep, buildSearchInput(task), context, AgentToolNames.KNOWLEDGE_SEARCH);
        if (searchStep.getStatus() == com.tuoman.ai_task_orchestrator.enums.AgentTaskStepStatus.FAILED) {
            failTask(task, searchStep);
            throw BusinessException.agentStepFailed(searchStep.getErrorMessage());
        }

        noContext = Boolean.TRUE.equals(searchOutput.get("noContext"));
        List<AgentTaskCitationResponse> citations = toCitations(searchOutput, task.getCollectionId());
        saveCitations(taskId, citations);
        task.setRetrievalCount(citations.size());
        task.setCitationCount(citations.size());

        if (noContext) {
            completeNoContextWorkflow(task, steps, searchOutput);
            return;
        }

        AgentTaskStepEntity summaryStep = steps.get(1);
        Map<String, Object> summaryInput = buildSummaryInput(task, searchOutput);
        Map<String, Object> summaryOutput = executeToolStep(
                task,
                summaryStep,
                summaryInput,
                context,
                AgentToolNames.CONTEXT_SUMMARY
        );
        if (summaryStep.getStatus() == com.tuoman.ai_task_orchestrator.enums.AgentTaskStepStatus.FAILED) {
            failTask(task, summaryStep);
            throw BusinessException.agentStepFailed(summaryStep.getErrorMessage());
        }

        AgentTaskStepEntity reportStep = steps.get(2);
        executeFinalReportStep(task, reportStep, searchOutput, summaryOutput, citations, context);
        if (reportStep.getStatus() == com.tuoman.ai_task_orchestrator.enums.AgentTaskStepStatus.FAILED) {
            failTask(task, reportStep);
            throw BusinessException.agentFinalReportFailed(reportStep.getErrorMessage());
        }

        task.setStepCount(3);
        task.setToolExecutionCount(2);
        task.setFailedStepCount(0);
        task.setStatus(com.tuoman.ai_task_orchestrator.enums.AgentTaskStatus.COMPLETED);
        task.setCompletedAt(LocalDateTime.now());
        task.setErrorCode(null);
        task.setErrorMessage(null);
        agentTaskRepository.save(task);
        agentTaskEventRecorder.recordTaskCompleted(taskId);
    }

    private Map<String, Object> executeToolStep(
            AgentTaskEntity task,
            AgentTaskStepEntity step,
            Map<String, Object> input,
            ToolExecutionContext context,
            String toolName
    ) {
        agentTaskStepService.markRunning(step, input);
        agentTaskEventRecorder.recordToolExecutionStarted(task.getId(), toolName, step.getDisplayTitle());

        long started = System.nanoTime();
        try {
            AgentTool tool = agentToolRegistry.findToolOrThrow(toolName);
            ToolExecutionResult result = tool.execute(input, context);
            if (!result.success()) {
                String traceId = AgentTaskEventRecorder.newTraceId();
                agentTaskStepService.markFailed(
                        step,
                        result.errorCode(),
                        result.errorMessage(),
                        traceId,
                        result.durationMs()
                );
                agentTaskEventRecorder.recordToolExecutionFailed(
                        task.getId(),
                        toolName,
                        step.getDisplayTitle(),
                        result.errorCode(),
                        result.errorMessage(),
                        traceId
                );
                return Map.of();
            }
            agentTaskStepService.markCompleted(step, result.output(), result.durationMs());
            agentTaskEventRecorder.recordToolExecutionCompleted(
                    task.getId(),
                    toolName,
                    step.getDisplayTitle(),
                    result.durationMs()
            );
            return result.output();
        } catch (BusinessException exception) {
            String traceId = AgentTaskEventRecorder.newTraceId();
            agentTaskStepService.markFailed(
                    step,
                    exception.getErrorCode().name(),
                    exception.getMessage(),
                    traceId,
                    elapsed(started)
            );
            agentTaskEventRecorder.recordToolExecutionFailed(
                    task.getId(),
                    toolName,
                    step.getDisplayTitle(),
                    exception.getErrorCode().name(),
                    exception.getMessage(),
                    traceId
            );
            throw exception;
        }
    }

    private void executeFinalReportStep(
            AgentTaskEntity task,
            AgentTaskStepEntity step,
            Map<String, Object> searchOutput,
            Map<String, Object> summaryOutput,
            List<AgentTaskCitationResponse> citations,
            ToolExecutionContext context
    ) {
        Map<String, Object> input = new LinkedHashMap<>();
        input.put("taskObjective", task.getObjective());
        input.put("scopeLabel", context.scopeLabel());
        input.put("knowledgeSearch", compactToolOutput(searchOutput));
        input.put("contextSummary", compactToolOutput(summaryOutput));
        agentTaskStepService.markRunning(step, input);
        agentTaskEventRecorder.recordFinalReportStarted(task.getId());

        long started = System.nanoTime();
        String systemPrompt = agentTaskPromptBuilder.buildFinalReportSystemPrompt();
        String userPrompt = agentTaskPromptBuilder.buildFinalReportUserPrompt(
                task.getObjective(),
                context.scopeLabel(),
                searchOutput,
                summaryOutput,
                citations
        );

        LlmGenerateOptions options = new LlmGenerateOptions();
        options.setTaskId(task.getId());
        LlmGenerateResult llmResult = llmProvider.generate(systemPrompt, userPrompt, options);
        long durationMs = (System.nanoTime() - started) / 1_000_000;

        if (!llmResult.isSuccess() || llmResult.getContent() == null || llmResult.getContent().isBlank()) {
            String traceId = AgentTaskEventRecorder.newTraceId();
            String message = llmResult.getErrorMessage() == null ? "最终报告生成失败" : llmResult.getErrorMessage();
            agentTaskStepService.markFailed(step, ErrorCode.AGENT_FINAL_REPORT_FAILED.name(), message, traceId, durationMs);
            agentTaskEventRecorder.recordFinalReportFailed(task.getId(), message, traceId);
            return;
        }

        Map<String, Object> output = new LinkedHashMap<>();
        output.put("finalReport", llmResult.getContent());
        output.put("llmMetadata", Map.of(
                "provider", llmResult.getProvider(),
                "model", llmResult.getModel(),
                "inputTokens", llmResult.getInputTokens(),
                "outputTokens", llmResult.getOutputTokens(),
                "latencyMs", llmResult.getLatencyMs(),
                "finishReason", llmResult.getFinishReason()
        ));

        agentTaskStepService.markCompleted(step, output, durationMs);
        agentTaskEventRecorder.recordFinalReportCompleted(task.getId(), durationMs, output.get("llmMetadata"));

        task.setResult(llmResult.getContent());
        task.setLlmProvider(llmResult.getProvider());
        task.setLlmModel(llmResult.getModel());
        task.setInputTokens(llmResult.getInputTokens());
        task.setOutputTokens(llmResult.getOutputTokens());
        task.setFinalReportLatencyMs(durationMs);
    }

    private void completeNoContextWorkflow(AgentTaskEntity task, List<AgentTaskStepEntity> steps, Map<String, Object> searchOutput) {
        AgentTaskStepEntity summaryStep = steps.get(1);
        agentTaskStepService.markSkipped(summaryStep, Map.of("summary", "无可总结内容"), "无可用检索片段");

        AgentTaskStepEntity reportStep = steps.get(2);
        Map<String, Object> reportInput = Map.of(
                "noContext", true,
                "taskObjective", task.getObjective()
        );
        agentTaskStepService.markRunning(reportStep, reportInput);
        agentTaskEventRecorder.recordFinalReportStarted(task.getId());

        String result = NO_CONTEXT_RESULT;

        Map<String, Object> reportOutput = Map.of(
                "finalReport", result,
                "deterministic", true,
                "llmSkipped", true
        );
        agentTaskStepService.markCompleted(reportStep, reportOutput, 0L);
        agentTaskEventRecorder.recordFinalReportCompleted(task.getId(), 0L, Map.of("deterministic", true));

        task.setResult(result);
        task.setLlmProvider(null);
        task.setLlmModel(null);
        task.setInputTokens(null);
        task.setOutputTokens(null);
        task.setFinalReportLatencyMs(0L);
        task.setStepCount(3);
        task.setToolExecutionCount(1);
        task.setFailedStepCount(0);
        task.setStatus(com.tuoman.ai_task_orchestrator.enums.AgentTaskStatus.COMPLETED);
        task.setCompletedAt(LocalDateTime.now());
        agentTaskRepository.save(task);
        agentTaskEventRecorder.recordTaskCompleted(task.getId());
    }

    private void failTask(AgentTaskEntity task, AgentTaskStepEntity failedStep) {
        task.setStatus(com.tuoman.ai_task_orchestrator.enums.AgentTaskStatus.FAILED);
        task.setErrorCode(failedStep.getErrorCode());
        task.setErrorMessage(failedStep.getErrorMessage());
        task.setTraceId(failedStep.getTraceId());
        task.setCompletedAt(LocalDateTime.now());
        task.setFailedStepCount(1);
        task.setStepCount(3);
        agentTaskRepository.save(task);
        agentTaskEventRecorder.recordTaskFailed(
                task.getId(),
                AgentTaskStep.EXECUTION,
                failedStep.getErrorCode(),
                failedStep.getErrorMessage(),
                failedStep.getTraceId(),
                null
        );
    }

    private Map<String, Object> buildSearchInput(AgentTaskEntity task) {
        Map<String, Object> input = new LinkedHashMap<>();
        input.put("query", task.getObjective());
        input.put("collectionId", task.getCollectionId());
        input.put("topK", agentTaskProperties.getDefaultTopK());
        return input;
    }

    private Map<String, Object> buildSummaryInput(AgentTaskEntity task, Map<String, Object> searchOutput) {
        Map<String, Object> input = new LinkedHashMap<>(searchOutput);
        input.put("taskObjective", task.getObjective());
        return input;
    }

    private ToolExecutionContext buildContext(AgentTaskEntity task) {
        String scopeLabel = task.getCollectionId() == null
                ? "全部文档"
                : "指定知识库分组：" + (task.getCollectionName() == null ? task.getCollectionId() : task.getCollectionName());
        return new ToolExecutionContext(
                task.getId(),
                task.getTitle(),
                task.getObjective(),
                task.getCollectionId(),
                task.getCollectionName(),
                scopeLabel
        );
    }

    @SuppressWarnings("unchecked")
    private List<AgentTaskCitationResponse> toCitations(Map<String, Object> searchOutput, Long collectionId) {
        Object citations = searchOutput.get("citations");
        if (!(citations instanceof List<?> list) || list.isEmpty()) {
            return List.of();
        }
        List<AgentTaskCitationResponse> result = new ArrayList<>();
        for (Object item : list) {
            if (!(item instanceof Map<?, ?> map)) {
                continue;
            }
            result.add(new AgentTaskCitationResponse(
                    readInt(map.get("sourceIndex")),
                    readLong(map.get("documentId")),
                    map.get("documentTitle") == null ? null : String.valueOf(map.get("documentTitle")),
                    readLong(map.get("chunkId")),
                    readDouble(map.get("score")),
                    map.get("contentSnippet") == null ? null : String.valueOf(map.get("contentSnippet")),
                    collectionId
            ));
        }
        return result;
    }

    private void saveCitations(Long taskId, List<AgentTaskCitationResponse> citations) {
        agentTaskCitationRepository.deleteByTaskId(taskId);
        for (AgentTaskCitationResponse citation : citations) {
            AgentTaskCitationEntity entity = new AgentTaskCitationEntity();
            entity.setTaskId(taskId);
            entity.setSourceIndex(citation.getSourceIndex());
            entity.setDocumentId(citation.getDocumentId());
            entity.setDocumentTitle(citation.getDocumentTitle());
            entity.setChunkId(citation.getChunkId());
            entity.setScore(citation.getScore());
            entity.setContentSnippet(citation.getContentSnippet());
            entity.setCollectionId(citation.getCollectionId());
            agentTaskCitationRepository.save(entity);
        }
    }

    private Map<String, Object> compactToolOutput(Map<String, Object> output) {
        if (output == null) {
            return Map.of();
        }
        Map<String, Object> compact = new LinkedHashMap<>();
        output.forEach((key, value) -> {
            if ("matchedChunks".equals(key) && value instanceof List<?> list) {
                compact.put(key, list.size() + " chunks");
            } else if ("citations".equals(key) && value instanceof List<?> list) {
                compact.put(key, list.size() + " citations");
            } else {
                compact.put(key, value);
            }
        });
        return compact;
    }

    private int readInt(Object value) {
        if (value instanceof Number number) {
            return number.intValue();
        }
        return Integer.parseInt(String.valueOf(value));
    }

    private Long readLong(Object value) {
        if (value == null) {
            return null;
        }
        if (value instanceof Number number) {
            return number.longValue();
        }
        return Long.parseLong(String.valueOf(value));
    }

    private Double readDouble(Object value) {
        if (value == null) {
            return null;
        }
        if (value instanceof Number number) {
            return number.doubleValue();
        }
        return Double.parseDouble(String.valueOf(value));
    }

    private long elapsed(long started) {
        return (System.nanoTime() - started) / 1_000_000;
    }
}
