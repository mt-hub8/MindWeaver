package com.tuoman.ai_task_orchestrator.state;

import com.tuoman.ai_task_orchestrator.enums.TaskStatus;
import org.springframework.stereotype.Component;

import java.util.EnumSet;
import java.util.Map;
import java.util.Set;

@Component
public class TaskStateMachine {

    private static final Map<TaskStatus, Set<TaskStatus>> ALLOWED_TRANSITIONS = Map.of(
            TaskStatus.PENDING, EnumSet.of(TaskStatus.RUNNING, TaskStatus.CANCELLED),
            TaskStatus.RUNNING, EnumSet.of(TaskStatus.SUCCESS, TaskStatus.FAILED, TaskStatus.CANCELLED),
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