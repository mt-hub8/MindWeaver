package com.tuoman.ai_task_orchestrator.mq;

public interface AgentTaskMessagePublisher {

    void publish(AgentTaskMessage message);
}
