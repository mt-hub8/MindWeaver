package com.tuoman.ai_task_orchestrator.service;

import io.micrometer.core.instrument.simple.SimpleMeterRegistry;
import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;

class MetricsRecorderTest {

    private final SimpleMeterRegistry meterRegistry = new SimpleMeterRegistry();

    private final MetricsRecorder metricsRecorder = new MetricsRecorder(meterRegistry);

    @Test
    void outboxMetricsShouldIncrementCounters() {
        metricsRecorder.recordOutboxClaimSuccess();
        metricsRecorder.recordOutboxClaimConflict();
        metricsRecorder.recordOutboxDispatchSuccess();
        metricsRecorder.recordOutboxDispatchFailed();

        assertThat(meterRegistry.counter("outbox.claim.success.count").count()).isEqualTo(1);
        assertThat(meterRegistry.counter("outbox.claim.conflict.count").count()).isEqualTo(1);
        assertThat(meterRegistry.counter("outbox.dispatch.success.count").count()).isEqualTo(1);
        assertThat(meterRegistry.counter("outbox.dispatch.failed.count").count()).isEqualTo(1);
    }
}
