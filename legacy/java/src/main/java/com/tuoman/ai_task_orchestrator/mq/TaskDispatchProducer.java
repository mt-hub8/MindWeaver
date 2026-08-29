package com.tuoman.ai_task_orchestrator.mq;

import com.tuoman.ai_task_orchestrator.config.RabbitMQConfig;
import lombok.RequiredArgsConstructor;
import org.springframework.amqp.rabbit.core.RabbitTemplate;
import org.springframework.stereotype.Component;

@Component
@RequiredArgsConstructor
/**
 * Task MQ 生产者。
 *
 * 消息只携带 taskId，表示“请尝试执行这个任务”。
 * 任务输入、状态和幂等判断都回到数据库读取，避免 MQ payload 成为第二份事实来源。
 */
public class TaskDispatchProducer {

    private final RabbitTemplate rabbitTemplate;

    public void sendTaskCreatedMessage(Long taskId) {
        TaskDispatchMessage message = new TaskDispatchMessage(taskId);

        rabbitTemplate.convertAndSend(
                RabbitMQConfig.TASK_EXCHANGE,
                RabbitMQConfig.TASK_CREATED_ROUTING_KEY,
                message
        );
    }
}
