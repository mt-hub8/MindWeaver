package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.agent.AgentTaskDisplayTexts;
import com.tuoman.ai_task_orchestrator.agent.AgentTaskEventRecorder;
import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.dto.CreateAgentTaskRequest;
import com.tuoman.ai_task_orchestrator.dto.CreateAgentTaskResponse;
import com.tuoman.ai_task_orchestrator.dto.MemoryRequest;
import com.tuoman.ai_task_orchestrator.dto.MemoryResponse;
import com.tuoman.ai_task_orchestrator.dto.SaveAgentTaskMemoryRequest;
import com.tuoman.ai_task_orchestrator.entity.AgentTaskEntity;
import com.tuoman.ai_task_orchestrator.entity.KnowledgeCollectionEntity;
import com.tuoman.ai_task_orchestrator.enums.AgentTaskStatus;
import com.tuoman.ai_task_orchestrator.enums.MemoryScope;
import com.tuoman.ai_task_orchestrator.enums.MemorySourceType;
import com.tuoman.ai_task_orchestrator.enums.MemoryType;
import com.tuoman.ai_task_orchestrator.enums.MemoryVisibility;
import com.tuoman.ai_task_orchestrator.mq.AgentTaskMessage;
import com.tuoman.ai_task_orchestrator.mq.AgentTaskMessagePublisher;
import com.tuoman.ai_task_orchestrator.repository.AgentTaskRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

/**
 * Agent Task 创建服务。
 *
 * AgentTask 从 PENDING 入队，worker 拉取后进入 RUNNING，最终进入 COMPLETED 或 FAILED。
 * 创建时会生成固定 step plan，并记录事件，实际执行交给异步 AgentTaskExecutor。
 */
@Service
@RequiredArgsConstructor
public class AgentTaskService {

    private final AgentTaskRepository agentTaskRepository;

    private final CollectionService collectionService;

    private final AgentTaskMessagePublisher agentTaskMessagePublisher;

    private final AgentTaskEventRecorder agentTaskEventRecorder;

    private final AgentTaskStepService agentTaskStepService;

    @Autowired(required = false)
    private AgentProfileService agentProfileService;

    @Autowired(required = false)
    private MemoryService memoryService;

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
        if (request.getAgentProfileId() != null && agentProfileService != null) {
            agentProfileService.getEnabledProfile(request.getAgentProfileId());
        }

        // collectionId 是 Agent Task 的知识库 scope。
        // 后续工具调用必须继承该 scope，不能绕过用户选择的 collection。
        AgentTaskEntity task = new AgentTaskEntity();
        task.setTitle(request.getTitle().trim());
        task.setObjective(request.getObjective().trim());
        task.setCollectionId(request.getCollectionId());
        task.setCollectionName(collectionName);
        task.setAgentProfileId(request.getAgentProfileId());
        task.setStatus(AgentTaskStatus.PENDING);
        AgentTaskEntity saved = agentTaskRepository.save(task);

        agentTaskEventRecorder.recordTaskCreated(saved.getId());
        // 固定计划：TOOL_CALL(KnowledgeSearch) -> TOOL_CALL(ContextSummary) -> FINAL_REPORT。
        // 先落 step 再入队，便于前端展示任务计划和失败定位。
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

    @Transactional
    public MemoryResponse saveSummaryAsMemory(Long taskId, SaveAgentTaskMemoryRequest request) {
        if (request == null || !request.isConfirmed()) {
            throw BusinessException.validationError("保存任务总结为记忆前必须由用户确认");
        }
        if (memoryService == null) {
            throw BusinessException.internalError("记忆服务不可用");
        }
        AgentTaskEntity task = agentTaskRepository.findById(taskId)
                .orElseThrow(BusinessException::agentTaskNotFound);
        if (task.getStatus() != AgentTaskStatus.COMPLETED
                || task.getResult() == null
                || task.getResult().isBlank()) {
            throw BusinessException.validationError("只有已完成且有结果的任务可以保存总结");
        }
        MemoryRequest memory = new MemoryRequest();
        memory.setTitle(request.getTitle() == null || request.getTitle().isBlank()
                ? "任务总结：" + task.getTitle()
                : request.getTitle().trim());
        memory.setContent(task.getResult());
        memory.setMemoryType(MemoryType.TASK_RESULT);
        memory.setMemoryScope(MemoryScope.TASK);
        memory.setVisibility(MemoryVisibility.PRIVATE);
        memory.setSourceType(MemorySourceType.AGENT_TASK);
        memory.setSourceId(String.valueOf(taskId));
        memory.setTaskId(taskId);
        memory.setAgentProfileId(task.getAgentProfileId());
        memory.setConfidence(request.getConfidence() == null ? 0.8 : request.getConfidence());
        memory.setImportance(request.getImportance() == null ? 60 : request.getImportance());
        memory.setMetadataJson("{\"userConfirmed\":true}");
        return memoryService.createMemory(memory);
    }
}
