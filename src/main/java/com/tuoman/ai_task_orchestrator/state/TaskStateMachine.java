package com.tuoman.ai_task_orchestrator.state;

import com.tuoman.ai_task_orchestrator.enums.TaskStatus;
import org.springframework.stereotype.Component;

import java.util.EnumSet;
import java.util.Map;
import java.util.Set;

@Component
/**
 * V0.3 Task 状态机。
 *
 * Task 当前状态是调度、查询、重试、取消共同依赖的事实来源。
 * SUCCESS、FAILED、CANCELLED 是终态，普通执行线程不能再把它们改回 RUNNING。
 */
public class TaskStateMachine {

    private static final Map<TaskStatus, Set<TaskStatus>> ALLOWED_TRANSITIONS = Map.of(
            TaskStatus.PENDING, EnumSet.of(TaskStatus.RUNNING, TaskStatus.CANCELLED),
            TaskStatus.RUNNING, EnumSet.of(TaskStatus.SUCCESS, TaskStatus.RETRY_PENDING, TaskStatus.FAILED, TaskStatus.CANCELLED),
            TaskStatus.RETRY_PENDING, EnumSet.of(TaskStatus.RUNNING, TaskStatus.FAILED, TaskStatus.CANCELLED),
            TaskStatus.SUCCESS, EnumSet.noneOf(TaskStatus.class),
            TaskStatus.FAILED, EnumSet.noneOf(TaskStatus.class),
            TaskStatus.CANCELLED, EnumSet.noneOf(TaskStatus.class)
    );

    public boolean canTransit(TaskStatus fromStatus, TaskStatus toStatus) {
        if (fromStatus == null || toStatus == null) {
            return false;
        }

        if (fromStatus == toStatus) {
            return true;
        }

        return ALLOWED_TRANSITIONS
                .getOrDefault(fromStatus, EnumSet.noneOf(TaskStatus.class))
                .contains(toStatus);
    }
}
