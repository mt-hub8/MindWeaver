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
