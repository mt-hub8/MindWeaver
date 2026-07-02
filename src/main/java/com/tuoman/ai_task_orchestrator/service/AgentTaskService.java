package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.agent.AgentTaskDisplayTexts;
import com.tuoman.ai_task_orchestrator.agent.AgentTaskEventRecorder;
import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.dto.CreateAgentTaskRequest;
import com.tuoman.ai_task_orchestrator.dto.CreateAgentTaskResponse;
import com.tuoman.ai_task_orchestrator.entity.AgentTaskEntity;
import com.tuoman.ai_task_orchestrator.entity.KnowledgeCollectionEntity;
import com.tuoman.ai_task_orchestrator.enums.AgentTaskStatus;
import com.tuoman.ai_task_orchestrator.mq.AgentTaskMessage;
import com.tuoman.ai_task_orchestrator.mq.AgentTaskMessagePublisher;
import com.tuoman.ai_task_orchestrator.repository.AgentTaskRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

@Service
@RequiredArgsConstructor
public class AgentTaskService {

    private final AgentTaskRepository agentTaskRepository;

    private final CollectionService collectionService;

    private final AgentTaskMessagePublisher agentTaskMessagePublisher;

    private final AgentTaskEventRecorder agentTaskEventRecorder;

    private final AgentTaskStepService agentTaskStepService;

    @Transactional
    public CreateAgentTaskResponse createTask(CreateAgentTaskRequest request) {
        if (request == null) {
            throw BusinessException.validationError("request must not be null");
        }
        if (request.getTitle() == null || request.getTitle().isBlank()) {
            throw BusinessException.validationError("任务标题不能为空");
        }
        if (request.getObjective() == null || request.getObjective().isBlank()) {
            throw BusinessException.validationError("任务目标不能为空");
        }

        String collectionName = null;
        if (request.getCollectionId() != null) {
            KnowledgeCollectionEntity collection = collectionService.findCollectionOrThrow(request.getCollectionId());
            collectionName = collection.getName();
        }

        AgentTaskEntity task = new AgentTaskEntity();
        task.setTitle(request.getTitle().trim());
        task.setObjective(request.getObjective().trim());
        task.setCollectionId(request.getCollectionId());
        task.setCollectionName(collectionName);
        task.setStatus(AgentTaskStatus.PENDING);
        AgentTaskEntity saved = agentTaskRepository.save(task);

        agentTaskEventRecorder.recordTaskCreated(saved.getId());
        agentTaskStepService.createFixedPlan(saved.getId());
        agentTaskEventRecorder.recordStepPlanCreated(saved.getId());

        try {
            agentTaskMessagePublisher.publish(new AgentTaskMessage(saved.getId()));
            agentTaskEventRecorder.recordTaskQueued(saved.getId());
        } catch (RuntimeException exception) {
            String traceId = AgentTaskEventRecorder.newTraceId();
            saved.setStatus(AgentTaskStatus.FAILED);
            saved.setErrorCode(com.tuoman.ai_task_orchestrator.common.error.ErrorCode.INTERNAL_ERROR.name());
            saved.setErrorMessage("任务入队失败，请稍后重试");
            saved.setTraceId(traceId);
            agentTaskRepository.save(saved);
            agentTaskEventRecorder.recordTaskFailed(
                    saved.getId(),
                    com.tuoman.ai_task_orchestrator.enums.AgentTaskStep.QUEUED,
                    com.tuoman.ai_task_orchestrator.common.error.ErrorCode.INTERNAL_ERROR.name(),
                    saved.getErrorMessage(),
                    traceId,
                    exception
            );
            throw BusinessException.internalError("AI 任务入队失败，请稍后重试");
        }

        return new CreateAgentTaskResponse(
                saved.getId(),
                saved.getStatus().name(),
                AgentTaskDisplayTexts.displayStatus(saved.getStatus()),
                "AI 任务已创建，系统将在后台按固定工具流程检索知识库、总结上下文并生成最终报告。"
        );
    }
}
