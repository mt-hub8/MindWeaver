package com.tuoman.ai_task_orchestrator.mq;

import com.tuoman.ai_task_orchestrator.config.RabbitMQConfig;
import com.tuoman.ai_task_orchestrator.service.AgentTaskExecutor;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.amqp.rabbit.annotation.RabbitListener;
import org.springframework.stereotype.Component;

@Slf4j
@Component
@RequiredArgsConstructor
public class AgentTaskConsumer {

    private final AgentTaskExecutor agentTaskExecutor;

    @RabbitListener(queues = RabbitMQConfig.AGENT_TASK_QUEUE)
    public void handleAgentTask(AgentTaskMessage message) {
        log.info("Received agent task message, taskId={}", message.getTaskId());
        agentTaskExecutor.execute(message.getTaskId());
    }
}
