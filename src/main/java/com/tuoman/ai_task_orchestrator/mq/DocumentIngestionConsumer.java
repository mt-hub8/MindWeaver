package com.tuoman.ai_task_orchestrator.mq;

import com.tuoman.ai_task_orchestrator.config.RabbitMQConfig;
import com.tuoman.ai_task_orchestrator.service.DocumentIngestionTaskHandler;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.amqp.rabbit.annotation.RabbitListener;
import org.springframework.stereotype.Component;

@Slf4j
@Component
@RequiredArgsConstructor
public class DocumentIngestionConsumer {

    private final DocumentIngestionTaskHandler documentIngestionTaskHandler;

    @RabbitListener(queues = RabbitMQConfig.DOCUMENT_INGESTION_QUEUE)
    public void handleDocumentIngestion(DocumentIngestionMessage message) {
        log.info(
                "Received document ingestion message, taskId={}, documentId={}",
                message.getTaskId(),
                message.getDocumentId()
        );
        documentIngestionTaskHandler.process(message.getTaskId());
    }
}
