package com.tuoman.ai_task_orchestrator.scheduler;

import com.tuoman.ai_task_orchestrator.entity.TaskEntity;
import com.tuoman.ai_task_orchestrator.enums.TaskStatus;
import com.tuoman.ai_task_orchestrator.repository.TaskRepository;
import com.tuoman.ai_task_orchestrator.service.TaskService;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;

import java.time.LocalDateTime;
import java.util.List;

@Slf4j
@Component
@RequiredArgsConstructor
/**
 * V0.10 timeout 调度器。
 *
 * 后台扫描超时 RUNNING 任务，防止执行线程卡死导致任务永久停留在运行中。
 * 它只能失败仍处于 RUNNING 的任务，不能覆盖已成功或已取消的终态。
 */
public class TaskTimeoutScheduler {

    private final TaskRepository taskRepository;

    private final TaskService taskService;

    @Scheduled(fixedDelay = 5000)
    public void scanTimedOutTasks() {
        List<TaskEntity> tasks = taskRepository.findTop20ByStatusAndTimeoutAtLessThanEqualOrderByTimeoutAtAsc(
                TaskStatus.RUNNING,
                LocalDateTime.now()
        );

        log.info("Found timed out running tasks count={}", tasks.size());

        for (TaskEntity task : tasks) {
            try {
                log.info("Mark task timed out, taskId={}", task.getId());
                taskService.tryMarkTaskTimedOut(task.getId());
            } catch (Exception e) {
                log.error("Failed to mark task timed out, taskId={}", task.getId(), e);
            }
        }
    }
}
