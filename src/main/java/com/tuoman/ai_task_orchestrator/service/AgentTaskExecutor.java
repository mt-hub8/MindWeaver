package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.agent.AgentTaskEventRecorder;
import com.tuoman.ai_task_orchestrator.agent.AgentTaskPromptBuilder;
import com.tuoman.ai_task_orchestrator.agent.AgentTaskProperties;
import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.common.error.ErrorCode;
import com.tuoman.ai_task_orchestrator.dto.AgentTaskCitationResponse;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingProvider;
import com.tuoman.ai_task_orchestrator.entity.AgentTaskCitationEntity;
import com.tuoman.ai_task_orchestrator.entity.AgentTaskEntity;
import com.tuoman.ai_task_orchestrator.enums.AgentTaskStatus;
import com.tuoman.ai_task_orchestrator.enums.AgentTaskStep;
import com.tuoman.ai_task_orchestrator.llm.LlmGenerateOptions;
import com.tuoman.ai_task_orchestrator.llm.LlmGenerateResult;
import com.tuoman.ai_task_orchestrator.llm.LlmProvider;
import com.tuoman.ai_task_orchestrator.repository.AgentTaskCitationRepository;
import com.tuoman.ai_task_orchestrator.repository.AgentTaskRepository;
import com.tuoman.ai_task_orchestrator.retrieval.CollectionAskScope;
import com.tuoman.ai_task_orchestrator.retrieval.RetrievalScope;
import com.tuoman.ai_task_orchestrator.rerank.RagTwoStageRetrievalService;
import com.tuoman.ai_task_orchestrator.rerank.RagTwoStageRetrievalService.RagRetrievalOutcome;
import com.tuoman.ai_task_orchestrator.rerank.RagTwoStageRetrievalService.RagRetrievedChunk;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.LocalDateTime;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;

@Slf4j
@Service
@RequiredArgsConstructor
public class AgentTaskExecutor {

    private static final int CONTENT_SNIPPET_MAX = 400;

    private static final String NO_CONTEXT_RESULT = "当前范围内没有可用于执行任务的知识库内容。";

    private final AgentTaskRepository agentTaskRepository;

    private final AgentTaskCitationRepository agentTaskCitationRepository;

    private final AgentTaskEventRecorder agentTaskEventRecorder;

    private final CollectionScopeService collectionScopeService;

    private final RagTwoStageRetrievalService ragTwoStageRetrievalService;

    private final AgentTaskPromptBuilder agentTaskPromptBuilder;

    private final LlmProvider llmProvider;

    private final EmbeddingProvider embeddingProvider;

    private final AgentTaskProperties agentTaskProperties;

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
            runExecution(task);
        } catch (BusinessException exception) {
            markFailed(task, exception.getErrorCode(), exception.getMessage(), AgentTaskEventRecorder.newTraceId());
            throw exception;
        } catch (RuntimeException exception) {
            markFailed(
                    task,
                    ErrorCode.AGENT_TASK_EXECUTION_FAILED,
                    exception.getMessage() == null ? "Agent task execution failed" : exception.getMessage(),
                    AgentTaskEventRecorder.newTraceId()
            );
            throw exception;
        }
    }

    private void runExecution(AgentTaskEntity task) {
        Long taskId = task.getId();
        CollectionAskScope askScope = resolveAskScope(task.getCollectionId());

        if (askScope.shouldSkipRetrieval()) {
            completeNoContext(task, askScope.noContextMessage());
            return;
        }

        RetrievalScope retrievalScope = toRetrievalScope(task.getCollectionId(), askScope);
        agentTaskEventRecorder.recordRetrievalStarted(taskId);
        long retrievalStarted = System.nanoTime();

        RagRetrievalOutcome outcome;
        try {
            outcome = ragTwoStageRetrievalService.retrieve(
                    task.getObjective(),
                    agentTaskProperties.getDefaultTopK(),
                    retrievalScope
            );
        } catch (RuntimeException exception) {
            throw BusinessException.agentTaskRetrievalFailed(
                    exception.getMessage() == null ? "Retrieval failed" : exception.getMessage()
            );
        }

        List<AgentTaskCitationResponse> citations = toCitations(outcome.chunks(), task.getCollectionId());
        long retrievalDurationMs = (System.nanoTime() - retrievalStarted) / 1_000_000;
        agentTaskEventRecorder.recordRetrievalCompleted(taskId, citations.size(), retrievalDurationMs);

        task.setRetrievalCount(outcome.chunks().size());
        saveCitations(taskId, citations);
        task.setCitationCount(citations.size());

        if (citations.isEmpty()) {
            completeNoContext(task, NO_CONTEXT_RESULT);
            return;
        }

        String scopeLabel = buildScopeLabel(task);
        String systemPrompt = agentTaskPromptBuilder.buildSystemPrompt();
        String userPrompt = agentTaskPromptBuilder.buildUserPrompt(task.getObjective(), scopeLabel, citations);

        agentTaskEventRecorder.recordLlmStarted(taskId);
        long llmStarted = System.nanoTime();

        LlmGenerateOptions options = new LlmGenerateOptions();
        options.setTaskId(taskId);
        LlmGenerateResult llmResult = llmProvider.generate(systemPrompt, userPrompt, options);

        long llmDurationMs = (System.nanoTime() - llmStarted) / 1_000_000;

        if (!llmResult.isSuccess() || llmResult.getContent() == null || llmResult.getContent().isBlank()) {
            throw BusinessException.agentTaskLlmFailed(
                    llmResult.getErrorMessage() == null ? "LLM generation failed" : llmResult.getErrorMessage()
            );
        }

        agentTaskEventRecorder.recordLlmCompleted(
                taskId,
                llmDurationMs,
                Map.of(
                        "provider", llmResult.getProvider(),
                        "model", llmResult.getModel(),
                        "inputTokens", llmResult.getInputTokens(),
                        "outputTokens", llmResult.getOutputTokens()
                )
        );

        task.setResult(llmResult.getContent());
        task.setLlmProvider(llmResult.getProvider());
        task.setLlmModel(llmResult.getModel());
        task.setInputTokens(llmResult.getInputTokens());
        task.setOutputTokens(llmResult.getOutputTokens());
        task.setStatus(AgentTaskStatus.COMPLETED);
        task.setCompletedAt(LocalDateTime.now());
        task.setErrorCode(null);
        task.setErrorMessage(null);
        agentTaskRepository.save(task);
        agentTaskEventRecorder.recordTaskCompleted(taskId);
    }

    private void completeNoContext(AgentTaskEntity task, String message) {
        task.setResult(message);
        task.setCitationCount(0);
        task.setRetrievalCount(0);
        task.setStatus(AgentTaskStatus.COMPLETED);
        task.setCompletedAt(LocalDateTime.now());
        task.setLlmProvider(null);
        task.setLlmModel(null);
        task.setInputTokens(null);
        task.setOutputTokens(null);
        agentTaskRepository.save(task);
        agentTaskEventRecorder.recordTaskCompleted(task.getId());
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

    private CollectionAskScope resolveAskScope(Long collectionId) {
        if (collectionId == null) {
            return CollectionAskScope.notApplicable();
        }
        try {
            return collectionScopeService.resolveForAsk(collectionId);
        } catch (BusinessException exception) {
            if (exception.getErrorCode() == ErrorCode.COLLECTION_NOT_FOUND) {
                throw BusinessException.agentTaskCollectionNotFound();
            }
            throw exception;
        }
    }

    private RetrievalScope toRetrievalScope(Long collectionId, CollectionAskScope askScope) {
        if (collectionId == null) {
            return RetrievalScope.allDocuments();
        }
        return RetrievalScope.collection(
                askScope.collectionId(),
                askScope.collectionName(),
                askScope.askableDocumentIds()
        );
    }

    private String buildScopeLabel(AgentTaskEntity task) {
        if (task.getCollectionId() == null) {
            return "全部文档";
        }
        return "指定知识库分组：" + (task.getCollectionName() == null ? task.getCollectionId() : task.getCollectionName());
    }

    private List<AgentTaskCitationResponse> toCitations(List<RagRetrievedChunk> chunks, Long collectionId) {
        List<AgentTaskCitationResponse> citations = new ArrayList<>();
        for (RagRetrievedChunk chunk : chunks) {
            citations.add(new AgentTaskCitationResponse(
                    chunk.rerankedRank(),
                    chunk.documentId(),
                    chunk.documentTitle(),
                    chunk.chunkId(),
                    chunk.rerankScore() != null ? chunk.rerankScore() : chunk.originalScore(),
                    contentSnippet(chunk.content()),
                    collectionId
            ));
        }
        return citations;
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

    private String contentSnippet(String content) {
        if (content == null) {
            return "";
        }
        if (content.length() <= CONTENT_SNIPPET_MAX) {
            return content;
        }
        return content.substring(0, CONTENT_SNIPPET_MAX);
    }
}
