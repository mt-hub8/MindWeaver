package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.entity.TaskAttemptEntity;
import com.tuoman.ai_task_orchestrator.enums.TaskAttemptStatus;
import com.tuoman.ai_task_orchestrator.llm.LlmResponse;
import com.tuoman.ai_task_orchestrator.repository.TaskAttemptRepository;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.net.InetAddress;
import java.time.LocalDateTime;
import java.util.List;

@Slf4j
@Service
@RequiredArgsConstructor
public class TaskAttemptService {

    private final TaskAttemptRepository taskAttemptRepository;

    @Transactional
    public TaskAttemptEntity createRunningAttempt(Long taskId) {
        int nextAttemptNo = taskAttemptRepository.findMaxAttemptNoByTaskId(taskId).orElse(0) + 1;

        TaskAttemptEntity attempt = new TaskAttemptEntity();
        attempt.setTaskId(taskId);
        attempt.setAttemptNo(nextAttemptNo);
        attempt.setStatus(TaskAttemptStatus.RUNNING);
        attempt.setWorkerId(resolveWorkerId());
        attempt.setStartedAt(LocalDateTime.now());

        TaskAttemptEntity savedAttempt = taskAttemptRepository.save(attempt);
        log.info(
                "task_attempt_created taskId={} attemptId={} attemptNo={} status={} workerId={}",
                taskId,
                savedAttempt.getId(),
                savedAttempt.getAttemptNo(),
                savedAttempt.getStatus(),
                savedAttempt.getWorkerId()
        );
        return savedAttempt;
    }

    @Transactional
    public TaskAttemptEntity markSuccess(
            Long attemptId,
            LlmResponse response,
            String renderedPrompt,
            String promptTemplateCode
    ) {
        TaskAttemptEntity attempt = findAttemptOrThrow(attemptId);
        attempt.setStatus(TaskAttemptStatus.SUCCESS);
        applyLlmMetadata(attempt, response, renderedPrompt, promptTemplateCode);
        attempt.setErrorMessage(null);
        attempt.setFinishedAt(LocalDateTime.now());

        TaskAttemptEntity savedAttempt = taskAttemptRepository.save(attempt);
        log.info(
                "task_attempt_finished taskId={} attemptId={} attemptNo={} status={} provider={} model={} latencyMs={}",
                savedAttempt.getTaskId(),
                savedAttempt.getId(),
                savedAttempt.getAttemptNo(),
                savedAttempt.getStatus(),
                savedAttempt.getLlmProvider(),
                savedAttempt.getLlmModel(),
                savedAttempt.getLlmLatencyMs()
        );
        return savedAttempt;
    }

    @Transactional
    public TaskAttemptEntity markFailed(
            Long attemptId,
            String errorMessage,
            LlmResponse response,
            String renderedPrompt,
            String promptTemplateCode
    ) {
        TaskAttemptEntity attempt = findAttemptOrThrow(attemptId);
        attempt.setStatus(TaskAttemptStatus.FAILED);
        applyLlmMetadata(attempt, response, renderedPrompt, promptTemplateCode);
        attempt.setErrorMessage(normalizeErrorMessage(errorMessage));
        attempt.setFinishedAt(LocalDateTime.now());

        TaskAttemptEntity savedAttempt = taskAttemptRepository.save(attempt);
        log.info(
                "task_attempt_finished taskId={} attemptId={} attemptNo={} status={} provider={} model={} latencyMs={} errorMessage={}",
                savedAttempt.getTaskId(),
                savedAttempt.getId(),
                savedAttempt.getAttemptNo(),
                savedAttempt.getStatus(),
                savedAttempt.getLlmProvider(),
                savedAttempt.getLlmModel(),
                savedAttempt.getLlmLatencyMs(),
                savedAttempt.getErrorMessage()
        );
        return savedAttempt;
    }

    @Transactional
    public TaskAttemptEntity markCancelled(Long attemptId, String message) {
        TaskAttemptEntity attempt = findAttemptOrThrow(attemptId);
        attempt.setStatus(TaskAttemptStatus.CANCELLED);
        attempt.setErrorMessage(normalizeErrorMessage(message));
        attempt.setFinishedAt(LocalDateTime.now());
        TaskAttemptEntity savedAttempt = taskAttemptRepository.save(attempt);
        log.info(
                "task_attempt_finished taskId={} attemptId={} attemptNo={} status={} errorMessage={}",
                savedAttempt.getTaskId(),
                savedAttempt.getId(),
                savedAttempt.getAttemptNo(),
                savedAttempt.getStatus(),
                savedAttempt.getErrorMessage()
        );
        return savedAttempt;
    }

    @Transactional(readOnly = true)
    public List<TaskAttemptEntity> getAttempts(Long taskId) {
        return taskAttemptRepository.findByTaskIdOrderByAttemptNoAsc(taskId);
    }

    private void applyLlmMetadata(
            TaskAttemptEntity attempt,
            LlmResponse response,
            String renderedPrompt,
            String promptTemplateCode
    ) {
        attempt.setRenderedPrompt(renderedPrompt);
        attempt.setPromptTemplateCode(promptTemplateCode);

        if (response == null) {
            return;
        }

        attempt.setLlmProvider(response.getProvider());
        attempt.setLlmModel(response.getModel());
        attempt.setPromptTokenCount(response.getPromptTokenCount());
        attempt.setCompletionTokenCount(response.getCompletionTokenCount());
        attempt.setTotalTokenCount(response.getTotalTokenCount());
        attempt.setLlmLatencyMs(response.getLatencyMs());
    }

    private TaskAttemptEntity findAttemptOrThrow(Long attemptId) {
        return taskAttemptRepository.findById(attemptId)
                .orElseThrow(() -> new IllegalArgumentException("Task attempt not found: " + attemptId));
    }

    private String normalizeErrorMessage(String errorMessage) {
        if (errorMessage == null || errorMessage.isBlank()) {
            return "Unknown task attempt error";
        }
        return errorMessage.length() > 2000 ? errorMessage.substring(0, 2000) : errorMessage;
    }

    private String resolveWorkerId() {
        try {
            return InetAddress.getLocalHost().getHostName();
        } catch (Exception e) {
            return "local-worker";
        }
    }
}
