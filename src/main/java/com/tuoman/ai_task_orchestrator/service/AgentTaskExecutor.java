package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.agent.AgentTaskEventRecorder;
import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.common.error.ErrorCode;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingProvider;
import com.tuoman.ai_task_orchestrator.entity.AgentTaskEntity;
import com.tuoman.ai_task_orchestrator.enums.AgentTaskStatus;
import com.tuoman.ai_task_orchestrator.enums.AgentTaskStep;
import com.tuoman.ai_task_orchestrator.repository.AgentTaskRepository;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.LocalDateTime;

/**
 * Agent Task 异步执行入口。
 *
 * 该类由 MQ worker 调用，把任务从 PENDING 推进到 RUNNING，
 * 并委托 AgentTaskWorkflowService 执行工具工作流。
 *
 * 关键不变量：失败必须同时写 task 状态和事件 trace，便于恢复和排障。
 */
@Slf4j
@Service
@RequiredArgsConstructor
public class AgentTaskExecutor {

    private final AgentTaskRepository agentTaskRepository;

    private final AgentTaskEventRecorder agentTaskEventRecorder;

    private final AgentTaskWorkflowService agentTaskWorkflowService;

    private final EmbeddingProvider embeddingProvider;

    @Transactional
    public void execute(Long taskId) {
        AgentTaskEntity task = agentTaskRepository.findById(taskId)
                .orElseThrow(BusinessException::agentTaskNotFound);

        if (task.getStatus() == AgentTaskStatus.COMPLETED || task.getStatus() == AgentTaskStatus.FAILED) {
            log.info("Skip agent task execution, taskId={}, status={}", taskId, task.getStatus());
            return;
        }

        // RUNNING 阶段记录当前 embedding provider/model，便于解释 KnowledgeSearchTool 的检索环境。
        task.setStatus(AgentTaskStatus.RUNNING);
        task.setStartedAt(LocalDateTime.now());
        task.setEmbeddingProvider(embeddingProvider.provider());
        task.setEmbeddingModel(embeddingProvider.model());
        agentTaskRepository.save(task);
        agentTaskEventRecorder.recordTaskStarted(taskId);

        try {
            agentTaskWorkflowService.executeWorkflow(task);
        } catch (BusinessException exception) {
            if (task.getStatus() != AgentTaskStatus.FAILED) {
                markFailed(task, exception.getErrorCode(), exception.getMessage(), AgentTaskEventRecorder.newTraceId());
            }
            throw exception;
        } catch (RuntimeException exception) {
            if (task.getStatus() != AgentTaskStatus.FAILED) {
                markFailed(
                        task,
                        ErrorCode.AGENT_TASK_EXECUTION_FAILED,
                        exception.getMessage() == null ? "Agent task execution failed" : exception.getMessage(),
                        AgentTaskEventRecorder.newTraceId()
                );
            }
            throw exception;
        }
    }

    private void markFailed(AgentTaskEntity task, ErrorCode errorCode, String message, String traceId) {
        // task 失败是终态；step 失败信息会在 workflow 中保留，这里补齐 task 级摘要。
        task.setStatus(AgentTaskStatus.FAILED);
        task.setErrorCode(errorCode.name());
        task.setErrorMessage(message);
        task.setTraceId(traceId);
        task.setCompletedAt(LocalDateTime.now());
        agentTaskRepository.save(task);
        agentTaskEventRecorder.recordTaskFailed(
                task.getId(),
                AgentTaskStep.EXECUTION,
                errorCode.name(),
                message,
                traceId,
                null
        );
    }
}
