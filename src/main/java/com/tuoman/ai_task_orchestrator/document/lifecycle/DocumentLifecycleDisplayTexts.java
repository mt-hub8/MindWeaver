package com.tuoman.ai_task_orchestrator.document.lifecycle;

import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;

public final class DocumentLifecycleDisplayTexts {

    private DocumentLifecycleDisplayTexts() {
    }

    public static String displayStatus(DocumentLifecycleStatus status) {
        if (status == null) {
            return "未知状态";
        }
        return switch (status) {
            case ACTIVE -> "已启用";
            case DELETED -> "已删除";
        };
    }
}
