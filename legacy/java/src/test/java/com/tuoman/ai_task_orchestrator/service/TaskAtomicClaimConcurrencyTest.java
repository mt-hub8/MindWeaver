package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.entity.TaskEntity;
import com.tuoman.ai_task_orchestrator.enums.TaskStatus;
import com.tuoman.ai_task_orchestrator.repository.TaskRepository;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;

import java.time.LocalDateTime;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest
class TaskAtomicClaimConcurrencyTest {

    @Autowired
    private TaskRepository taskRepository;

    @Autowired
    private TaskService taskService;

    @Test
    void onlyOneConcurrentExecutorShouldClaimSamePendingTask() throws InterruptedException {
        TaskEntity task = new TaskEntity();
        task.setPrompt("atomic claim test");
        task.setStatus(TaskStatus.PENDING);
        task.setRetryCount(0);
        task.setMaxRetry(3);
        task.setNextRetryAt(null);
        task.setTimeoutSeconds(3600);
        task.setTimeoutAt(null);
        TaskEntity savedTask = taskRepository.save(task);

        CountDownLatch readyLatch = new CountDownLatch(2);
        CountDownLatch startLatch = new CountDownLatch(1);
        AtomicInteger successCount = new AtomicInteger(0);
        ExecutorService executorService = Executors.newFixedThreadPool(2);

        for (int i = 0; i < 2; i++) {
            executorService.submit(() -> {
                readyLatch.countDown();
                try {
                    startLatch.await(5, TimeUnit.SECONDS);
                    if (taskService.tryStartTaskExecution(savedTask.getId(), "任务开始执行")) {
                        successCount.incrementAndGet();
                    }
                } catch (InterruptedException e) {
                    Thread.currentThread().interrupt();
                }
            });
        }

        assertThat(readyLatch.await(5, TimeUnit.SECONDS)).isTrue();
        startLatch.countDown();
        executorService.shutdown();
        assertThat(executorService.awaitTermination(10, TimeUnit.SECONDS)).isTrue();

        TaskEntity claimedTask = taskRepository.findById(savedTask.getId()).orElseThrow();
        assertThat(successCount.get()).isEqualTo(1);
        assertThat(claimedTask.getStatus()).isEqualTo(TaskStatus.RUNNING);
        assertThat(claimedTask.getTimeoutAt()).isAfter(LocalDateTime.now().minusSeconds(1));

        taskRepository.deleteById(savedTask.getId());
    }
}
