package com.tuoman.ai_task_orchestrator.mq;

import com.tuoman.ai_task_orchestrator.config.RabbitMQConfig;
import lombok.RequiredArgsConstructor;
import org.springframework.amqp.rabbit.core.RabbitTemplate;
import org.springframework.stereotype.Component;

@Component
@RequiredArgsConstructor
public class RabbitAgentTaskMessagePublisher implements AgentTaskMessagePublisher {

    private final RabbitTemplate rabbitTemplate;

    @Override
    public void publish(AgentTaskMessage message) {
        rabbitTemplate.convertAndSend(
                RabbitMQConfig.TASK_EXCHANGE,
                RabbitMQConfig.AGENT_TASK_ROUTING_KEY,
                message
        );
    }
}
