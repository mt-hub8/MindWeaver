package com.tuoman.ai_task_orchestrator.mq;

public interface DocumentIngestionMessagePublisher {

    void publish(DocumentIngestionMessage message);
}
