package com.tuoman.ai_task_orchestrator.agent;

import com.tuoman.ai_task_orchestrator.enums.AgentTaskStepStatus;

public final class AgentTaskStepDisplayTexts {

    private AgentTaskStepDisplayTexts() {
    }

    public static String displayStatus(AgentTaskStepStatus status) {
        if (status == null) {
            return "未知";
        }
        return switch (status) {
            case PENDING -> "待执行";
            case RUNNING -> "执行中";
            case COMPLETED -> "已完成";
            case FAILED -> "执行失败";
            case SKIPPED -> "已跳过";
        };
    }
}
