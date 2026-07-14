package com.tuoman.ai_task_orchestrator.service;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.tuoman.ai_task_orchestrator.entity.TaskOutboxEntity;
import com.tuoman.ai_task_orchestrator.enums.TaskOutboxStatus;
import com.tuoman.ai_task_orchestrator.mq.TaskDispatchProducer;
import com.tuoman.ai_task_orchestrator.repository.TaskOutboxRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.lang.management.ManagementFactory;
import java.time.LocalDateTime;
import java.util.List;
import java.util.Optional;

@Service
@RequiredArgsConstructor
/**
 * Task outbox 派发服务。
 *
 * 负责 claim due outbox、解析 payload、发送 RabbitMQ 消息并记录 SENT/FAILED。
 * outbox 状态和 MQ 消息都不是 Task 当前状态事实来源；Consumer 仍必须回查 task。
 */
public class TaskOutboxDispatchService {

    private static final int LOCK_STALE_SECONDS = 60;

    private static final int RETRY_DELAY_SECONDS = 30;

    private final TaskOutboxRepository taskOutboxRepository;

    private final TaskDispatchProducer taskDispatchProducer;

    private final ObjectMapper objectMapper;

    private final MetricsRecorder metricsRecorder;

    private final String dispatcherId = ManagementFactory.getRuntimeMXBean().getName();

    @Transactional
    public Optional<TaskOutboxEntity> claimDueOutbox(Long outboxId) {
        // 多实例 scheduler 可能同时扫描到同一 outbox。
        // claim 必须是原子更新，失败即视为其他 dispatcher 已接管。
        LocalDateTime now = LocalDateTime.now();
        int claimed = taskOutboxRepository.claimOutbox(
                outboxId,
                TaskOutboxStatus.PROCESSING,
                List.of(TaskOutboxStatus.PENDING, TaskOutboxStatus.FAILED),
                dispatcherId,
                now,
                now,
                now.minusSeconds(LOCK_STALE_SECONDS)
        );

        if (claimed != 1) {
            metricsRecorder.recordOutboxClaimConflict();
            return Optional.empty();
        }

        metricsRecorder.recordOutboxClaimSuccess();
        return taskOutboxRepository.findById(outboxId);
    }

    public Long sendOutboxMessage(TaskOutboxEntity outbox) {
        if (!TaskOutboxService.EVENT_TYPE_TASK_DISPATCH_REQUESTED.equals(outbox.getEventType())) {
            throw new IllegalStateException("Unsupported task outbox event type: " + outbox.getEventType());
        }

        Long taskId = extractTaskId(outbox.getPayload());
        taskDispatchProducer.sendTaskCreatedMessage(taskId);
        return taskId;
    }

    @Transactional
    public void markSent(Long outboxId) {
        taskOutboxRepository.markSent(
                outboxId,
                TaskOutboxStatus.PROCESSING,
                TaskOutboxStatus.SENT,
                LocalDateTime.now()
        );
        metricsRecorder.recordOutboxDispatchSuccess();
    }

    @Transactional
    public void markFailed(Long outboxId, Exception exception) {
        // MQ 投递失败只让 outbox 延迟重试，不直接修改 Task 状态。
        // Task 是否执行仍由后续成功投递和 Consumer claim 决定。
        taskOutboxRepository.markFailed(
                outboxId,
                TaskOutboxStatus.PROCESSING,
                TaskOutboxStatus.FAILED,
                LocalDateTime.now().plusSeconds(RETRY_DELAY_SECONDS),
                normalizeErrorMessage(exception.getMessage()),
                LocalDateTime.now()
        );
        metricsRecorder.recordOutboxDispatchFailed();
    }

    private Long extractTaskId(String payload) {
        try {
            JsonNode root = objectMapper.readTree(payload);
            JsonNode taskIdNode = root.get("taskId");
            if (taskIdNode == null || !taskIdNode.canConvertToLong()) {
                throw new IllegalArgumentException("Missing taskId in outbox payload");
            }
            return taskIdNode.asLong();
        } catch (Exception e) {
            throw new IllegalArgumentException("Invalid task outbox payload", e);
        }
    }

    private String normalizeErrorMessage(String message) {
        if (message == null || message.isBlank()) {
            return "Unknown outbox dispatch error";
        }
        return message.length() > 2000 ? message.substring(0, 2000) : message;
    }
}
