package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.entity.TaskOutboxEntity;
import com.tuoman.ai_task_orchestrator.enums.TaskOutboxStatus;
import com.tuoman.ai_task_orchestrator.repository.TaskOutboxRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

@Service
@RequiredArgsConstructor
/**
 * V0.5 之后的 Task transactional outbox 写入服务。
 *
 * 创建 Task 时在同一个数据库事务中写入 outbox，后续由 scheduler 异步投递 MQ。
 * 这样可以避免“Task 已提交但 MQ 发送失败”造成任务永远没人执行。
 */
public class TaskOutboxService {

    public static final String AGGREGATE_TYPE_TASK = "TASK";

    public static final String EVENT_TYPE_TASK_DISPATCH_REQUESTED = "TASK_DISPATCH_REQUESTED";

    private final TaskOutboxRepository taskOutboxRepository;

    @Transactional
    public TaskOutboxEntity createTaskDispatchOutbox(Long taskId) {
        TaskOutboxEntity outbox = new TaskOutboxEntity();
        outbox.setAggregateType(AGGREGATE_TYPE_TASK);
        outbox.setAggregateId(taskId);
        outbox.setEventType(EVENT_TYPE_TASK_DISPATCH_REQUESTED);
        outbox.setPayload("{\"taskId\":" + taskId + "}");
        outbox.setStatus(TaskOutboxStatus.PENDING);
        outbox.setRetryCount(0);
        outbox.setNextRetryAt(null);
        return taskOutboxRepository.save(outbox);
    }
}
