package com.tuoman.ai_task_orchestrator.service;

import io.micrometer.core.instrument.MeterRegistry;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Component;

@Component
@RequiredArgsConstructor
public class MetricsRecorder {

    private final MeterRegistry meterRegistry;

    public void recordOutboxClaimSuccess() {
        increment("outbox.claim.success.count");
    }

    public void recordOutboxClaimConflict() {
        increment("outbox.claim.conflict.count");
    }

    public void recordOutboxDispatchSuccess() {
        increment("outbox.dispatch.success.count");
    }

    public void recordOutboxDispatchFailed() {
        increment("outbox.dispatch.failed.count");
    }

    private void increment(String metricName) {
        meterRegistry.counter(metricName).increment();
    }
}
