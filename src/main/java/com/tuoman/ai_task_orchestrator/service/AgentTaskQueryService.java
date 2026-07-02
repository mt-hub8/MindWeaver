package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.agent.AgentTaskDisplayTexts;
import com.tuoman.ai_task_orchestrator.agent.AgentTaskEventDisplayTexts;
import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.dto.AgentTaskCitationResponse;
import com.tuoman.ai_task_orchestrator.dto.AgentTaskDetailResponse;
import com.tuoman.ai_task_orchestrator.dto.AgentTaskEventResponse;
import com.tuoman.ai_task_orchestrator.dto.AgentTaskModelMetadataResponse;
import com.tuoman.ai_task_orchestrator.dto.AgentTaskSummaryResponse;
import com.tuoman.ai_task_orchestrator.entity.AgentTaskCitationEntity;
import com.tuoman.ai_task_orchestrator.entity.AgentTaskEntity;
import com.tuoman.ai_task_orchestrator.entity.AgentTaskEventEntity;
import com.tuoman.ai_task_orchestrator.repository.AgentTaskCitationRepository;
import com.tuoman.ai_task_orchestrator.repository.AgentTaskEventRepository;
import com.tuoman.ai_task_orchestrator.repository.AgentTaskRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.List;

@Service
@RequiredArgsConstructor
public class AgentTaskQueryService {

    private final AgentTaskRepository agentTaskRepository;

    private final AgentTaskEventRepository agentTaskEventRepository;

    private final AgentTaskCitationRepository agentTaskCitationRepository;

    @Transactional(readOnly = true)
    public List<AgentTaskSummaryResponse> listTasks() {
        return agentTaskRepository.findTop20ByOrderByCreatedAtDesc()
                .stream()
                .map(this::toSummary)
                .toList();
    }

    @Transactional(readOnly = true)
    public AgentTaskDetailResponse getTask(Long taskId) {
        AgentTaskEntity task = findTaskOrThrow(taskId);
        List<AgentTaskCitationResponse> citations = agentTaskCitationRepository.findByTaskIdOrderBySourceIndexAsc(taskId)
                .stream()
                .map(this::toCitation)
                .toList();
        return new AgentTaskDetailResponse(
                task.getId(),
                task.getTitle(),
                task.getObjective(),
                task.getStatus().name(),
                AgentTaskDisplayTexts.displayStatus(task.getStatus()),
                task.getCollectionId(),
                task.getCollectionName(),
                buildScopeLabel(task),
                task.getResult(),
                task.getErrorCode(),
                task.getErrorMessage(),
                task.getTraceId(),
                new AgentTaskModelMetadataResponse(
                        task.getLlmProvider(),
                        task.getLlmModel(),
                        task.getEmbeddingProvider(),
                        task.getEmbeddingModel(),
                        task.getInputTokens(),
                        task.getOutputTokens(),
                        task.getRetrievalCount(),
                        task.getCitationCount()
                ),
                citations,
                task.getCreatedAt(),
                task.getStartedAt(),
                task.getCompletedAt()
        );
    }

    @Transactional(readOnly = true)
    public List<AgentTaskEventResponse> listEvents(Long taskId) {
        findTaskOrThrow(taskId);
        return agentTaskEventRepository.findByTaskIdOrderByCreatedAtAsc(taskId)
                .stream()
                .map(this::toEvent)
                .toList();
    }

    private AgentTaskEntity findTaskOrThrow(Long taskId) {
        return agentTaskRepository.findById(taskId)
                .orElseThrow(BusinessException::agentTaskNotFound);
    }

    private AgentTaskSummaryResponse toSummary(AgentTaskEntity task) {
        return new AgentTaskSummaryResponse(
                task.getId(),
                task.getTitle(),
                task.getStatus().name(),
                AgentTaskDisplayTexts.displayStatus(task.getStatus()),
                task.getCollectionId(),
                task.getCollectionName(),
                task.getCitationCount(),
                task.getCreatedAt(),
                task.getCompletedAt()
        );
    }

    private AgentTaskCitationResponse toCitation(AgentTaskCitationEntity entity) {
        return new AgentTaskCitationResponse(
                entity.getSourceIndex(),
                entity.getDocumentId(),
                entity.getDocumentTitle(),
                entity.getChunkId(),
                entity.getScore(),
                entity.getContentSnippet(),
                entity.getCollectionId()
        );
    }

    private AgentTaskEventResponse toEvent(AgentTaskEventEntity entity) {
        return new AgentTaskEventResponse(
                entity.getId(),
                entity.getEventType().name(),
                AgentTaskEventDisplayTexts.displayEventType(entity.getEventType()),
                entity.getStep() == null ? null : entity.getStep().name(),
                entity.getStatus().name(),
                displayEventStatus(entity.getStatus().name()),
                entity.getMessage(),
                entity.getDisplayMessage(),
                entity.getDurationMs(),
                entity.getErrorCode(),
                entity.getErrorMessage(),
                entity.getTraceId(),
                entity.getCreatedAt()
        );
    }

    private String displayEventStatus(String status) {
        return switch (status) {
            case "STARTED" -> "进行中";
            case "COMPLETED" -> "已完成";
            case "FAILED" -> "失败";
            default -> status;
        };
    }

    private String buildScopeLabel(AgentTaskEntity task) {
        if (task.getCollectionId() == null) {
            return "全部文档";
        }
        return "指定知识库分组：" + (task.getCollectionName() == null ? task.getCollectionId() : task.getCollectionName());
    }
}
