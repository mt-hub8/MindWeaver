package com.tuoman.ai_task_orchestrator.mq;

import com.tuoman.ai_task_orchestrator.config.RabbitMQConfig;
import com.tuoman.ai_task_orchestrator.service.TaskExecutionService;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.amqp.rabbit.annotation.RabbitListener;
import org.springframework.stereotype.Component;

@Slf4j
@Component
@RequiredArgsConstructor
/**
 * Task MQ 消费者。
 *
 * RabbitMQ 至少一次投递可能带来重复消息，因此 Consumer 只把 taskId 交给
 * TaskExecutionService，真正的幂等 claim 和状态检查在 service 层完成。
 */
public class TaskDispatchConsumer {

    private final TaskExecutionService taskExecutionService;

    @RabbitListener(queues = RabbitMQConfig.TASK_CREATED_QUEUE)
    public void handleTaskCreated(TaskDispatchMessage message) {
        log.info("Received task dispatch message, taskId={}", message.getTaskId());

        taskExecutionService.executeTask(message.getTaskId());
    }
}
