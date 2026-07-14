package com.tuoman.ai_task_orchestrator.scheduler;

import com.tuoman.ai_task_orchestrator.entity.TaskEntity;
import com.tuoman.ai_task_orchestrator.enums.TaskStatus;
import com.tuoman.ai_task_orchestrator.repository.TaskRepository;
import com.tuoman.ai_task_orchestrator.service.TaskOutboxService;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;
import org.springframework.transaction.annotation.Transactional;

import java.time.LocalDateTime;
import java.util.List;

@Slf4j
@Component
@RequiredArgsConstructor
/**
 * V0.8 retry 调度器。
 *
 * 扫描到期的 RETRY_PENDING 任务并重新创建 dispatch outbox。
 * retry 不创建新 Task，也不能改变 taskId；它只是把同一任务重新送回执行链路。
 */
public class TaskRetryScheduler {

    private final TaskRepository taskRepository;

    private final TaskOutboxService taskOutboxService;

    @Transactional
    @Scheduled(fixedDelay = 5000)
    public void dispatchRetryTasks() {
        List<TaskEntity> tasks = taskRepository.findTop20ByStatusAndNextRetryAtLessThanEqualOrderByNextRetryAtAsc(
                TaskStatus.RETRY_PENDING,
                LocalDateTime.now()
        );

        log.info("Found retryable tasks count={}", tasks.size());

        for (TaskEntity task : tasks) {
            try {
                reserveRetryTaskAndCreateOutbox(task.getId());
            } catch (Exception e) {
                log.error("Failed to create retry task outbox, taskId={}", task.getId(), e);
            }
        }
    }

    @Transactional
    public void reserveRetryTaskAndCreateOutbox(Long taskId) {
        // reserve 会临时推迟 nextRetryAt，防止多个 scheduler 同时为同一 retry 创建 outbox。
        // 这与 Consumer 端的 atomic claim 共同构成重复执行保护。
        LocalDateTime now = LocalDateTime.now();
        LocalDateTime reservedUntil = now.plusSeconds(30);
        int reserved = taskRepository.reserveRetryDispatch(
                taskId,
                TaskStatus.RETRY_PENDING,
                now,
                reservedUntil
        );

        if (reserved != 1) {
            log.info("Skip retry task because reserve failed, taskId={}", taskId);
            return;
        }

        taskOutboxService.createTaskDispatchOutbox(taskId);
        log.info("Retry task outbox created, taskId={}, nextRetryAt postponed to {}", taskId, reservedUntil);
    }
}
