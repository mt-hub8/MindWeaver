package com.tuoman.ai_task_orchestrator.state;

import com.tuoman.ai_task_orchestrator.enums.TaskStatus;
import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;

class TaskStateMachineTest {

    private final TaskStateMachine taskStateMachine = new TaskStateMachine();

    @Test
    void shouldAllowMainValidTransitions() {
        assertThat(taskStateMachine.canTransit(TaskStatus.PENDING, TaskStatus.RUNNING)).isTrue();
        assertThat(taskStateMachine.canTransit(TaskStatus.RUNNING, TaskStatus.SUCCESS)).isTrue();
        assertThat(taskStateMachine.canTransit(TaskStatus.RUNNING, TaskStatus.FAILED)).isTrue();
        assertThat(taskStateMachine.canTransit(TaskStatus.RUNNING, TaskStatus.CANCELLED)).isTrue();
        assertThat(taskStateMachine.canTransit(TaskStatus.RUNNING, TaskStatus.RETRY_PENDING)).isTrue();
        assertThat(taskStateMachine.canTransit(TaskStatus.RETRY_PENDING, TaskStatus.RUNNING)).isTrue();
        assertThat(taskStateMachine.canTransit(TaskStatus.RETRY_PENDING, TaskStatus.FAILED)).isTrue();
    }

    @Test
    void shouldRejectInvalidTransitions() {
        assertThat(taskStateMachine.canTransit(TaskStatus.PENDING, TaskStatus.SUCCESS)).isFalse();
        assertThat(taskStateMachine.canTransit(TaskStatus.SUCCESS, TaskStatus.RUNNING)).isFalse();
        assertThat(taskStateMachine.canTransit(TaskStatus.FAILED, TaskStatus.RUNNING)).isFalse();
        assertThat(taskStateMachine.canTransit(TaskStatus.CANCELLED, TaskStatus.RUNNING)).isFalse();
    }

    @Test
    void shouldRejectTerminalStatusTransitionsToOtherStatuses() {
        for (TaskStatus terminalStatus : new TaskStatus[]{
                TaskStatus.SUCCESS,
                TaskStatus.FAILED,
                TaskStatus.CANCELLED
        }) {
            assertThat(taskStateMachine.canTransit(terminalStatus, TaskStatus.PENDING)).isFalse();
            assertThat(taskStateMachine.canTransit(terminalStatus, TaskStatus.RUNNING)).isFalse();
            assertThat(taskStateMachine.canTransit(terminalStatus, TaskStatus.RETRY_PENDING)).isFalse();
        }
    }

    @Test
    void shouldRejectNullStatuses() {
        assertThat(taskStateMachine.canTransit(null, TaskStatus.RUNNING)).isFalse();
        assertThat(taskStateMachine.canTransit(TaskStatus.PENDING, null)).isFalse();
    }
}
