package com.tuoman.ai_task_orchestrator.agent;

import com.tuoman.ai_task_orchestrator.enums.AgentTaskStatus;

public final class AgentTaskDisplayTexts {

    private AgentTaskDisplayTexts() {
    }

    public static String displayStatus(AgentTaskStatus status) {
        if (status == null) {
            return "未知";
        }
        return switch (status) {
            case PENDING -> "待处理";
            case RUNNING -> "执行中";
            case COMPLETED -> "已完成";
            case FAILED -> "执行失败";
            case CANCELED -> "已取消";
        };
    }
}
