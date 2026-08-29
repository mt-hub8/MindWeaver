package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.TaskOutboxEntity;
import com.tuoman.ai_task_orchestrator.enums.TaskOutboxStatus;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.data.domain.PageRequest;
import org.springframework.transaction.annotation.Transactional;

import java.time.LocalDateTime;
import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest(properties = "spring.task.scheduling.enabled=false")
@Transactional
class TaskOutboxRepositoryTest {

    @Autowired
    private TaskOutboxRepository taskOutboxRepository;

    @Test
    void pendingOutboxShouldBeClaimedOnlyOnceAndMarkedSent() {
        TaskOutboxEntity outbox = saveOutbox(TaskOutboxStatus.PENDING);
        LocalDateTime now = LocalDateTime.now();

        int firstClaim = taskOutboxRepository.claimOutbox(
                outbox.getId(),
                TaskOutboxStatus.PROCESSING,
                List.of(TaskOutboxStatus.PENDING, TaskOutboxStatus.FAILED),
                "dispatcher-a",
                now,
                now,
                now.minusSeconds(60)
        );
        int secondClaim = taskOutboxRepository.claimOutbox(
                outbox.getId(),
                TaskOutboxStatus.PROCESSING,
                List.of(TaskOutboxStatus.PENDING, TaskOutboxStatus.FAILED),
                "dispatcher-b",
                now,
                now,
                now.minusSeconds(60)
        );

        assertThat(firstClaim).isEqualTo(1);
        assertThat(secondClaim).isZero();

        int markedSent = taskOutboxRepository.markSent(
                outbox.getId(),
                TaskOutboxStatus.PROCESSING,
                TaskOutboxStatus.SENT,
                LocalDateTime.now()
        );

        assertThat(markedSent).isEqualTo(1);
        TaskOutboxEntity saved = taskOutboxRepository.findById(outbox.getId()).orElseThrow();
        assertThat(saved.getStatus()).isEqualTo(TaskOutboxStatus.SENT);
        assertThat(saved.getLockedBy()).isNull();
        assertThat(saved.getLockedAt()).isNull();
    }

    @Test
    void sentOutboxShouldNotBeClaimed() {
        TaskOutboxEntity outbox = saveOutbox(TaskOutboxStatus.SENT);
        LocalDateTime now = LocalDateTime.now();

        int claimed = taskOutboxRepository.claimOutbox(
                outbox.getId(),
                TaskOutboxStatus.PROCESSING,
                List.of(TaskOutboxStatus.PENDING, TaskOutboxStatus.FAILED),
                "dispatcher-a",
                now,
                now,
                now.minusSeconds(60)
        );

        assertThat(claimed).isZero();
    }

    @Test
    void markFailedShouldIncreaseRetryCountAndSetRetryMetadata() {
        TaskOutboxEntity outbox = saveOutbox(TaskOutboxStatus.PROCESSING);
        outbox.setRetryCount(1);
        outbox.setLockedBy("dispatcher-a");
        outbox.setLockedAt(LocalDateTime.now());
        taskOutboxRepository.saveAndFlush(outbox);

        LocalDateTime nextRetryAt = LocalDateTime.now().plusSeconds(30);
        int markedFailed = taskOutboxRepository.markFailed(
                outbox.getId(),
                TaskOutboxStatus.PROCESSING,
                TaskOutboxStatus.FAILED,
                nextRetryAt,
                "send failed",
                LocalDateTime.now()
        );

        assertThat(markedFailed).isEqualTo(1);
        TaskOutboxEntity saved = taskOutboxRepository.findById(outbox.getId()).orElseThrow();
        assertThat(saved.getStatus()).isEqualTo(TaskOutboxStatus.FAILED);
        assertThat(saved.getRetryCount()).isEqualTo(2);
        assertThat(saved.getNextRetryAt()).isBetween(nextRetryAt.minusSeconds(1), nextRetryAt.plusSeconds(1));
        assertThat(saved.getLastErrorMessage()).isEqualTo("send failed");
        assertThat(saved.getLockedBy()).isNull();
        assertThat(saved.getLockedAt()).isNull();
    }

    @Test
    void findDueOutboxesShouldSkipFutureRetryRows() {
        TaskOutboxEntity due = saveOutbox(TaskOutboxStatus.PENDING);
        TaskOutboxEntity future = saveOutbox(TaskOutboxStatus.FAILED);
        future.setNextRetryAt(LocalDateTime.now().plusMinutes(5));
        taskOutboxRepository.saveAndFlush(future);

        List<TaskOutboxEntity> dueOutboxes = taskOutboxRepository.findDueOutboxes(
                List.of(TaskOutboxStatus.PENDING, TaskOutboxStatus.FAILED),
                TaskOutboxStatus.PROCESSING,
                LocalDateTime.now(),
                LocalDateTime.now().minusSeconds(60),
                PageRequest.of(0, 20)
        );

        assertThat(dueOutboxes).extracting(TaskOutboxEntity::getId).contains(due.getId());
        assertThat(dueOutboxes).extracting(TaskOutboxEntity::getId).doesNotContain(future.getId());
    }

    @Test
    void failedOutboxShouldBeClaimedOnlyWhenDue() {
        TaskOutboxEntity dueFailed = saveOutbox(TaskOutboxStatus.FAILED);
        dueFailed.setNextRetryAt(LocalDateTime.now().minusSeconds(1));
        taskOutboxRepository.saveAndFlush(dueFailed);

        TaskOutboxEntity nonDueFailed = saveOutbox(TaskOutboxStatus.FAILED);
        nonDueFailed.setNextRetryAt(LocalDateTime.now().plusMinutes(5));
        taskOutboxRepository.saveAndFlush(nonDueFailed);

        LocalDateTime now = LocalDateTime.now();
        int dueClaimed = taskOutboxRepository.claimOutbox(
                dueFailed.getId(),
                TaskOutboxStatus.PROCESSING,
                List.of(TaskOutboxStatus.PENDING, TaskOutboxStatus.FAILED),
                "dispatcher-a",
                now,
                now,
                now.minusSeconds(60)
        );
        int nonDueClaimed = taskOutboxRepository.claimOutbox(
                nonDueFailed.getId(),
                TaskOutboxStatus.PROCESSING,
                List.of(TaskOutboxStatus.PENDING, TaskOutboxStatus.FAILED),
                "dispatcher-a",
                now,
                now,
                now.minusSeconds(60)
        );

        assertThat(dueClaimed).isEqualTo(1);
        assertThat(nonDueClaimed).isZero();
    }

    @Test
    void staleProcessingOutboxShouldBeClaimedAgain() {
        TaskOutboxEntity outbox = saveOutbox(TaskOutboxStatus.PROCESSING);
        outbox.setLockedBy("stale-dispatcher");
        outbox.setLockedAt(LocalDateTime.now().minusMinutes(2));
        taskOutboxRepository.saveAndFlush(outbox);

        LocalDateTime now = LocalDateTime.now();
        int claimed = taskOutboxRepository.claimOutbox(
                outbox.getId(),
                TaskOutboxStatus.PROCESSING,
                List.of(TaskOutboxStatus.PENDING, TaskOutboxStatus.FAILED),
                "dispatcher-b",
                now,
                now,
                now.minusSeconds(60)
        );

        assertThat(claimed).isEqualTo(1);
        TaskOutboxEntity saved = taskOutboxRepository.findById(outbox.getId()).orElseThrow();
        assertThat(saved.getStatus()).isEqualTo(TaskOutboxStatus.PROCESSING);
        assertThat(saved.getLockedBy()).isEqualTo("dispatcher-b");
        assertThat(saved.getLockedAt()).isNotNull();
    }

    @Test
    void nonStaleProcessingOutboxShouldNotBeClaimedAgain() {
        TaskOutboxEntity outbox = saveOutbox(TaskOutboxStatus.PROCESSING);
        outbox.setLockedBy("active-dispatcher");
        outbox.setLockedAt(LocalDateTime.now());
        taskOutboxRepository.saveAndFlush(outbox);

        LocalDateTime now = LocalDateTime.now();
        int claimed = taskOutboxRepository.claimOutbox(
                outbox.getId(),
                TaskOutboxStatus.PROCESSING,
                List.of(TaskOutboxStatus.PENDING, TaskOutboxStatus.FAILED),
                "dispatcher-b",
                now,
                now,
                now.minusSeconds(60)
        );

        assertThat(claimed).isZero();
    }

    @Test
    void findDueOutboxesShouldIncludeStaleProcessingRows() {
        TaskOutboxEntity stale = saveOutbox(TaskOutboxStatus.PROCESSING);
        stale.setLockedBy("stale-dispatcher");
        stale.setLockedAt(LocalDateTime.now().minusMinutes(2));
        taskOutboxRepository.saveAndFlush(stale);

        TaskOutboxEntity active = saveOutbox(TaskOutboxStatus.PROCESSING);
        active.setLockedBy("active-dispatcher");
        active.setLockedAt(LocalDateTime.now());
        taskOutboxRepository.saveAndFlush(active);

        List<TaskOutboxEntity> dueOutboxes = taskOutboxRepository.findDueOutboxes(
                List.of(TaskOutboxStatus.PENDING, TaskOutboxStatus.FAILED),
                TaskOutboxStatus.PROCESSING,
                LocalDateTime.now(),
                LocalDateTime.now().minusSeconds(60),
                PageRequest.of(0, 20)
        );

        assertThat(dueOutboxes).extracting(TaskOutboxEntity::getId).contains(stale.getId());
        assertThat(dueOutboxes).extracting(TaskOutboxEntity::getId).doesNotContain(active.getId());
    }

    private TaskOutboxEntity saveOutbox(TaskOutboxStatus status) {
        TaskOutboxEntity outbox = new TaskOutboxEntity();
        outbox.setAggregateType("TASK");
        outbox.setAggregateId(1L);
        outbox.setEventType("TASK_DISPATCH_REQUESTED");
        outbox.setPayload("{\"taskId\":1}");
        outbox.setStatus(status);
        outbox.setRetryCount(0);
        return taskOutboxRepository.saveAndFlush(outbox);
    }
}
