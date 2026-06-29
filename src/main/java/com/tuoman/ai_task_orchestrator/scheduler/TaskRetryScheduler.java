package com.tuoman.ai_task_orchestrator.scheduler;

import com.tuoman.ai_task_orchestrator.entity.TaskEntity;
import com.tuoman.ai_task_orchestrator.enums.TaskStatus;
import com.tuoman.ai_task_orchestrator.mq.TaskDispatchProducer;
import com.tuoman.ai_task_orchestrator.repository.TaskRepository;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;

import java.time.LocalDateTime;
import java.util.List;

@Slf4j
@Component
@RequiredArgsConstructor
public class TaskRetryScheduler {

    private final TaskRepository taskRepository;

    private final TaskDispatchProducer taskDispatchProducer;

    @Scheduled(fixedDelay = 5000)
    public void dispatchRetryTasks() {
        List<TaskEntity> tasks = taskRepository.findTop20ByStatusAndNextRetryAtLessThanEqualOrderByNextRetryAtAsc(
                TaskStatus.RETRY_PENDING,
                LocalDateTime.now()
        );

        log.info("Found retryable tasks count={}", tasks.size());

        for (TaskEntity task : tasks) {
            log.info("Resend retry task message, taskId={}", task.getId());
            taskDispatchProducer.sendTaskCreatedMessage(task.getId());
        }
    }
}
