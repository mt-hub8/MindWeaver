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

@Service
@RequiredArgsConstructor
public class AgentTaskStepService {

    private final AgentTaskStepRepository agentTaskStepRepository;

    private final AgentWorkflowJsonCodec agentWorkflowJsonCodec;

    @Transactional
    public List<AgentTaskStepEntity> createFixedPlan(Long taskId) {
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
