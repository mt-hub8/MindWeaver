package com.tuoman.ai_task_orchestrator.service;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.tuoman.ai_task_orchestrator.entity.TaskOutboxEntity;
import com.tuoman.ai_task_orchestrator.enums.TaskOutboxStatus;
import com.tuoman.ai_task_orchestrator.mq.TaskDispatchProducer;
import com.tuoman.ai_task_orchestrator.repository.TaskOutboxRepository;
import io.micrometer.core.instrument.simple.SimpleMeterRegistry;
import org.junit.jupiter.api.Test;
import org.springframework.transaction.annotation.Transactional;

import java.lang.reflect.Method;
import java.time.LocalDateTime;
import java.util.Optional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyCollection;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class TaskOutboxDispatchServiceTest {

    private final TaskOutboxRepository taskOutboxRepository = mock(TaskOutboxRepository.class);

    private final TaskDispatchProducer taskDispatchProducer = mock(TaskDispatchProducer.class);

    private final SimpleMeterRegistry meterRegistry = new SimpleMeterRegistry();

    private final MetricsRecorder metricsRecorder = new MetricsRecorder(meterRegistry);

    private final TaskOutboxDispatchService taskOutboxDispatchService = new TaskOutboxDispatchService(
            taskOutboxRepository,
            taskDispatchProducer,
            new ObjectMapper(),
            metricsRecorder
    );

    @Test
    void claimSuccessShouldReturnOutboxAndRecordMetric() {
        TaskOutboxEntity outbox = outbox(1L, 101L);
        when(taskOutboxRepository.claimOutbox(
                eq(1L),
                eq(TaskOutboxStatus.PROCESSING),
                anyCollection(),
                anyString(),
                any(LocalDateTime.class),
                any(LocalDateTime.class),
                any(LocalDateTime.class)
        )).thenReturn(1);
        when(taskOutboxRepository.findById(1L)).thenReturn(Optional.of(outbox));

        Optional<TaskOutboxEntity> claimed = taskOutboxDispatchService.claimDueOutbox(1L);

        assertThat(claimed).contains(outbox);
        assertThat(meterRegistry.counter("outbox.claim.success.count").count()).isEqualTo(1);
    }

    @Test
    void claimFailureShouldReturnEmptyAndRecordMetric() {
        when(taskOutboxRepository.claimOutbox(
                eq(1L),
                eq(TaskOutboxStatus.PROCESSING),
                anyCollection(),
                anyString(),
                any(LocalDateTime.class),
                any(LocalDateTime.class),
                any(LocalDateTime.class)
        )).thenReturn(0);

        Optional<TaskOutboxEntity> claimed = taskOutboxDispatchService.claimDueOutbox(1L);

        assertThat(claimed).isEmpty();
        verify(taskOutboxRepository, never()).findById(1L);
        assertThat(meterRegistry.counter("outbox.claim.conflict.count").count()).isEqualTo(1);
    }

    @Test
    void sendOutboxMessageShouldSendRabbitMqOutsideTransactionalMethod() throws Exception {
        TaskOutboxEntity outbox = outbox(2L, 102L);

        Long taskId = taskOutboxDispatchService.sendOutboxMessage(outbox);

        assertThat(taskId).isEqualTo(102L);
        verify(taskDispatchProducer).sendTaskCreatedMessage(102L);
        assertThat(method("sendOutboxMessage", TaskOutboxEntity.class).getAnnotation(Transactional.class)).isNull();
    }

    @Test
    void markSentShouldUpdateOutboxAndRecordMetric() {
        taskOutboxDispatchService.markSent(3L);

        verify(taskOutboxRepository).markSent(
                eq(3L),
                eq(TaskOutboxStatus.PROCESSING),
                eq(TaskOutboxStatus.SENT),
                any(LocalDateTime.class)
        );
        assertThat(meterRegistry.counter("outbox.dispatch.success.count").count()).isEqualTo(1);
    }

    @Test
    void markFailedShouldUpdateOutboxAndRecordMetric() {
        taskOutboxDispatchService.markFailed(4L, new RuntimeException("rabbit down"));

        verify(taskOutboxRepository).markFailed(
                eq(4L),
                eq(TaskOutboxStatus.PROCESSING),
                eq(TaskOutboxStatus.FAILED),
                any(LocalDateTime.class),
                eq("rabbit down"),
                any(LocalDateTime.class)
        );
        assertThat(meterRegistry.counter("outbox.dispatch.failed.count").count()).isEqualTo(1);
    }

    @Test
    void transactionalBoundaryAnnotationsShouldBeShortDatabaseStepsOnly() throws Exception {
        assertThat(method("claimDueOutbox", Long.class).getAnnotation(Transactional.class)).isNotNull();
        assertThat(method("markSent", Long.class).getAnnotation(Transactional.class)).isNotNull();
        assertThat(method("markFailed", Long.class, Exception.class).getAnnotation(Transactional.class)).isNotNull();
        assertThat(method("sendOutboxMessage", TaskOutboxEntity.class).getAnnotation(Transactional.class)).isNull();
    }

    private Method method(String name, Class<?>... parameterTypes) throws NoSuchMethodException {
        return TaskOutboxDispatchService.class.getDeclaredMethod(name, parameterTypes);
    }

    private TaskOutboxEntity outbox(Long id, Long taskId) {
        TaskOutboxEntity outbox = new TaskOutboxEntity();
        outbox.setId(id);
        outbox.setAggregateType("TASK");
        outbox.setAggregateId(taskId);
        outbox.setEventType("TASK_DISPATCH_REQUESTED");
        outbox.setPayload("{\"taskId\":" + taskId + "}");
        outbox.setStatus(TaskOutboxStatus.PROCESSING);
        outbox.setRetryCount(0);
        return outbox;
    }
}
