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
                taskService.markTaskTimedOut(task.getId());
            } catch (Exception e) {
                log.error("Failed to mark task timed out, taskId={}", task.getId(), e);
            }
        }
    }
}
