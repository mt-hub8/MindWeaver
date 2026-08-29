package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.agent.AgentWorkflowJsonCodec;
import com.tuoman.ai_task_orchestrator.agent.tool.AgentToolNames;
import com.tuoman.ai_task_orchestrator.entity.AgentTaskStepEntity;
import com.tuoman.ai_task_orchestrator.enums.AgentTaskStepStatus;
import com.tuoman.ai_task_orchestrator.enums.AgentTaskStepType;
import com.tuoman.ai_task_orchestrator.repository.AgentTaskStepRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.LocalDateTime;
import java.util.List;
import java.util.Map;

/**
 * Agent Task Step 状态服务。
 *
 * Step 承载 TOOL_CALL、LLM_GENERATION/FINAL_REPORT 等执行单元的输入、输出、耗时和错误。
 * 它让 Agent Task 的多步过程可恢复、可展示、可审计。
 */
@Service
@RequiredArgsConstructor
public class AgentTaskStepService {

    private final AgentTaskStepRepository agentTaskStepRepository;

    private final AgentWorkflowJsonCodec agentWorkflowJsonCodec;

    @Transactional
    public List<AgentTaskStepEntity> createFixedPlan(Long taskId) {
        // 固定计划避免 LLM planner 随意发明工具链。
        // 规则型 workflow 的边界清晰，便于记录每一步是否成功。
        if (agentTaskStepRepository.existsByTaskId(taskId)) {
            return agentTaskStepRepository.findByTaskIdOrderByStepOrderAsc(taskId);
        }
        saveStep(taskId, 1, AgentTaskStepType.TOOL_CALL, AgentToolNames.KNOWLEDGE_SEARCH,
                "检索知识库", "检索知识库");
        saveStep(taskId, 2, AgentTaskStepType.TOOL_CALL, AgentToolNames.CONTEXT_SUMMARY,
                "总结检索结果", "总结检索结果");
        saveStep(taskId, 3, AgentTaskStepType.FINAL_REPORT, AgentToolNames.FINAL_REPORT,
                "生成最终报告", "生成最终报告");
        return agentTaskStepRepository.findByTaskIdOrderByStepOrderAsc(taskId);
    }

    @Transactional(readOnly = true)
    public List<AgentTaskStepEntity> listSteps(Long taskId) {
        return agentTaskStepRepository.findByTaskIdOrderByStepOrderAsc(taskId);
    }

    @Transactional
    public AgentTaskStepEntity markRunning(AgentTaskStepEntity step, Map<String, Object> input) {
        step.setStatus(AgentTaskStepStatus.RUNNING);
        step.setStartedAt(LocalDateTime.now());
        step.setInputJson(agentWorkflowJsonCodec.serialize(input));
        step.setErrorCode(null);
        step.setErrorMessage(null);
        step.setTraceId(null);
        return agentTaskStepRepository.save(step);
    }

    @Transactional
    public AgentTaskStepEntity markCompleted(AgentTaskStepEntity step, Map<String, Object> output, long durationMs) {
        step.setStatus(AgentTaskStepStatus.COMPLETED);
        step.setCompletedAt(LocalDateTime.now());
        step.setDurationMs(durationMs);
        step.setOutputJson(agentWorkflowJsonCodec.serialize(output));
        return agentTaskStepRepository.save(step);
    }

    @Transactional
    public AgentTaskStepEntity markSkipped(AgentTaskStepEntity step, Map<String, Object> output, String reason) {
        step.setStatus(AgentTaskStepStatus.SKIPPED);
        step.setStartedAt(step.getStartedAt() == null ? LocalDateTime.now() : step.getStartedAt());
        step.setCompletedAt(LocalDateTime.now());
        step.setDurationMs(0L);
        if (output == null) {
            output = Map.of("reason", reason == null ? "已跳过" : reason);
        }
        step.setOutputJson(agentWorkflowJsonCodec.serialize(output));
        return agentTaskStepRepository.save(step);
    }

    @Transactional
    public AgentTaskStepEntity markFailed(
            AgentTaskStepEntity step,
            String errorCode,
            String errorMessage,
            String traceId,
            long durationMs
    ) {
        // step 失败时保留 traceId；task 终态由 workflow/executor 汇总设置。
        step.setStatus(AgentTaskStepStatus.FAILED);
        step.setCompletedAt(LocalDateTime.now());
        step.setDurationMs(durationMs);
        step.setErrorCode(errorCode);
        step.setErrorMessage(errorMessage);
        step.setTraceId(traceId);
        return agentTaskStepRepository.save(step);
    }

    private void saveStep(
            Long taskId,
            int order,
            AgentTaskStepType type,
            String toolName,
            String title,
            String displayTitle
    ) {
        AgentTaskStepEntity step = new AgentTaskStepEntity();
        step.setTaskId(taskId);
        step.setStepOrder(order);
        step.setStepType(type);
        step.setToolName(toolName);
        step.setTitle(title);
        step.setDisplayTitle(displayTitle);
        step.setStatus(AgentTaskStepStatus.PENDING);
        agentTaskStepRepository.save(step);
    }
}
