package com.tuoman.ai_task_orchestrator.scheduler;

import com.tuoman.ai_task_orchestrator.entity.TaskOutboxEntity;
import com.tuoman.ai_task_orchestrator.enums.TaskOutboxStatus;
import com.tuoman.ai_task_orchestrator.repository.TaskOutboxRepository;
import com.tuoman.ai_task_orchestrator.service.TaskOutboxDispatchService;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.data.domain.PageRequest;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;

import java.time.LocalDateTime;
import java.util.List;
import java.util.Optional;

@Slf4j
@Component
@RequiredArgsConstructor
/**
 * Task outbox 后台扫描器。
 *
 * V0.5 的 MQ 派发从同步发送演进为定时扫描 outbox，提升创建任务与消息投递的一致性。
 * 这里只负责投递触发信号，不执行任务本身。
 */
public class TaskOutboxDispatcherScheduler {

    private static final int BATCH_SIZE = 20;

    private static final int LOCK_STALE_SECONDS = 60;

    private final TaskOutboxRepository taskOutboxRepository;

    private final TaskOutboxDispatchService taskOutboxDispatchService;

    @Scheduled(fixedDelay = 5000)
    public void dispatchDueOutboxes() {
        LocalDateTime now = LocalDateTime.now();
        List<TaskOutboxEntity> outboxes = taskOutboxRepository.findDueOutboxes(
                List.of(TaskOutboxStatus.PENDING, TaskOutboxStatus.FAILED),
                TaskOutboxStatus.PROCESSING,
                now,
                now.minusSeconds(LOCK_STALE_SECONDS),
                PageRequest.of(0, BATCH_SIZE)
        );

        log.info("Found due task outboxes count={}", outboxes.size());

        for (TaskOutboxEntity outbox : outboxes) {
            dispatchSingleOutbox(outbox.getId());
        }
    }

    public void dispatchSingleOutbox(Long outboxId) {
        Optional<TaskOutboxEntity> claimedOutbox = taskOutboxDispatchService.claimDueOutbox(outboxId);

        if (claimedOutbox.isEmpty()) {
            log.info("Skip task outbox because claim failed, outboxId={}", outboxId);
            return;
        }

        TaskOutboxEntity outbox = claimedOutbox.get();
        Long taskId = outbox.getAggregateId();

        try {
            log.info(
                    "Dispatch task outbox, outboxId={}, taskId={}, eventType={}",
                    outboxId,
                    taskId,
                    outbox.getEventType()
            );

            taskId = taskOutboxDispatchService.sendOutboxMessage(outbox);
            taskOutboxDispatchService.markSent(outboxId);
            log.info("Task outbox sent, outboxId={}, taskId={}", outboxId, taskId);
        } catch (Exception e) {
            taskOutboxDispatchService.markFailed(outboxId, e);
            log.error("Task outbox dispatch failed, outboxId={}, taskId={}", outboxId, taskId, e);
        }
    }
}
