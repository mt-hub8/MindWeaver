package com.tuoman.ai_task_orchestrator.agent;

import com.tuoman.ai_task_orchestrator.enums.AgentTaskEventType;

public final class AgentTaskEventDisplayTexts {

    private AgentTaskEventDisplayTexts() {
    }

    public static String displayEventType(AgentTaskEventType eventType) {
        if (eventType == null) {
            return "未知事件";
        }
        return switch (eventType) {
            case TASK_CREATED -> "任务已创建";
            case TASK_QUEUED -> "任务已入队";
            case TASK_STARTED -> "任务开始执行";
            case RETRIEVAL_STARTED -> "检索相关文档";
            case RETRIEVAL_COMPLETED -> "检索完成";
            case LLM_STARTED -> "调用大模型";
            case LLM_COMPLETED -> "大模型生成完成";
            case TASK_COMPLETED -> "任务完成";
            case TASK_FAILED -> "任务失败";
            case STEP_PLAN_CREATED -> "任务执行计划";
            case TOOL_EXECUTION_STARTED -> "工具执行开始";
            case TOOL_EXECUTION_COMPLETED -> "工具执行完成";
            case TOOL_EXECUTION_FAILED -> "工具执行失败";
            case FINAL_REPORT_STARTED -> "生成最终报告";
            case FINAL_REPORT_COMPLETED -> "最终报告完成";
            case FINAL_REPORT_FAILED -> "最终报告失败";
        };
    }
}
