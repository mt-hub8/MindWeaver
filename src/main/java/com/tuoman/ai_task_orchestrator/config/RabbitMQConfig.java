package com.tuoman.ai_task_orchestrator.config;

import org.springframework.amqp.core.Binding;
import org.springframework.amqp.core.BindingBuilder;
import org.springframework.amqp.core.DirectExchange;
import org.springframework.amqp.core.Queue;
import org.springframework.amqp.core.QueueBuilder;
import org.springframework.amqp.rabbit.config.SimpleRabbitListenerContainerFactory;
import org.springframework.amqp.rabbit.connection.ConnectionFactory;
import org.springframework.amqp.rabbit.core.RabbitTemplate;
import org.springframework.amqp.support.converter.Jackson2JsonMessageConverter;
import org.springframework.amqp.support.converter.MessageConverter;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration
/**
 * RabbitMQ 基础设施配置。
 *
 * V0.5 引入的异步任务派发、后续文档 ingestion 和 Agent Task 都复用同一个 exchange。
 * queue/routing-key 是触发后台 worker 的基础设施约定，不承载业务事实状态。
 */
public class RabbitMQConfig {

    public static final String TASK_EXCHANGE = "task.exchange";

    public static final String TASK_CREATED_QUEUE = "task.created.queue";

    public static final String TASK_CREATED_ROUTING_KEY = "task.created";

    public static final String DOCUMENT_INGESTION_QUEUE = "document.ingestion.queue";

    public static final String DOCUMENT_INGESTION_ROUTING_KEY = "document.ingestion";

    public static final String AGENT_TASK_QUEUE = "agent.task.queue";

    public static final String AGENT_TASK_ROUTING_KEY = "agent.task";

    @Bean
    public DirectExchange taskExchange() {
        return new DirectExchange(TASK_EXCHANGE);
    }

    @Bean
    public Queue taskCreatedQueue() {
        return QueueBuilder.durable(TASK_CREATED_QUEUE).build();
    }

    @Bean
    public Binding taskCreatedBinding() {
        return BindingBuilder
                .bind(taskCreatedQueue())
                .to(taskExchange())
                .with(TASK_CREATED_ROUTING_KEY);
    }

    @Bean
    public Queue documentIngestionQueue() {
        return QueueBuilder.durable(DOCUMENT_INGESTION_QUEUE).build();
    }

    @Bean
    public Binding documentIngestionBinding() {
        return BindingBuilder
                .bind(documentIngestionQueue())
                .to(taskExchange())
                .with(DOCUMENT_INGESTION_ROUTING_KEY);
    }

    @Bean
    public Queue agentTaskQueue() {
        return QueueBuilder.durable(AGENT_TASK_QUEUE).build();
    }

    @Bean
    public Binding agentTaskBinding() {
        return BindingBuilder
                .bind(agentTaskQueue())
                .to(taskExchange())
                .with(AGENT_TASK_ROUTING_KEY);
    }

    @Bean
    public MessageConverter jsonMessageConverter() {
        return new Jackson2JsonMessageConverter();
    }

    @Bean
    public RabbitTemplate rabbitTemplate(
            ConnectionFactory connectionFactory,
            MessageConverter jsonMessageConverter
    ) {
        RabbitTemplate rabbitTemplate = new RabbitTemplate(connectionFactory);
        rabbitTemplate.setMessageConverter(jsonMessageConverter);
        return rabbitTemplate;
    }

    @Bean
    public SimpleRabbitListenerContainerFactory rabbitListenerContainerFactory(
            ConnectionFactory connectionFactory,
            MessageConverter jsonMessageConverter
    ) {
        SimpleRabbitListenerContainerFactory factory = new SimpleRabbitListenerContainerFactory();
        factory.setConnectionFactory(connectionFactory);
        factory.setMessageConverter(jsonMessageConverter);
        return factory;
    }
}
