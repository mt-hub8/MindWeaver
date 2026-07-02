package com.tuoman.ai_task_orchestrator.mq;

import com.tuoman.ai_task_orchestrator.config.RabbitMQConfig;
import lombok.RequiredArgsConstructor;
import org.springframework.amqp.rabbit.core.RabbitTemplate;
import org.springframework.stereotype.Component;

@Component
@RequiredArgsConstructor
public class RabbitDocumentIngestionMessagePublisher implements DocumentIngestionMessagePublisher {

    private final RabbitTemplate rabbitTemplate;

    @Override
    public void publish(DocumentIngestionMessage message) {
        rabbitTemplate.convertAndSend(
                RabbitMQConfig.TASK_EXCHANGE,
                RabbitMQConfig.DOCUMENT_INGESTION_ROUTING_KEY,
                message
        );
    }
}
