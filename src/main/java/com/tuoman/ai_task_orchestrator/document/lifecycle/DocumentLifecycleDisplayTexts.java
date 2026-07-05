package com.tuoman.ai_task_orchestrator.document.lifecycle;

import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;

public final class DocumentLifecycleDisplayTexts {

    private DocumentLifecycleDisplayTexts() {
    }

    public static String displayStatus(DocumentLifecycleStatus status) {
        if (status == null) {
            return "未知状态";
        }
        return switch (status) {
            case ACTIVE -> "已启用";
            case TRASHED -> "已放入垃圾箱";
            case PURGED -> "已永久删除";
        };
    }

    public static String displayProcessingStatus(DocumentStatus status) {
        if (status == null) {
            return "未知";
        }
        return switch (status) {
            case UPLOADED -> "已上传";
            case CHUNKED -> "已切分";
            case READY -> "索引就绪";
            case FAILED -> "处理失败";
        };
    }

    public static String lifecycleHint(
            DocumentLifecycleStatus lifecycleStatus,
            DocumentStatus processingStatus,
            boolean canAsk
    ) {
        if (lifecycleStatus == DocumentLifecycleStatus.TRASHED) {
            return "该文档已在垃圾箱，不会再用于知识库问答。可在垃圾箱中恢复或永久删除。";
        }
        if (lifecycleStatus == DocumentLifecycleStatus.PURGED) {
            return "该文档已永久删除，不可恢复。";
        }
        if (canAsk) {
            return "当前文档可以用于知识库问答。";
        }
        if (processingStatus == DocumentStatus.FAILED) {
            return "文档处理失败，暂不能用于知识库问答。请查看处理记录或重新处理。";
        }
        return "文档索引尚未就绪，请等待处理完成后再提问。";
    }

    public static String deleteSuccessMessage() {
        return "已放入垃圾箱：该文档不会再用于知识库问答，7 天内可恢复。";
    }

    public static String deleteAlreadyTrashedMessage() {
        return "该文档已在垃圾箱中，不会再用于知识库问答。";
    }

    public static String restoreSuccessMessage() {
        return "文档已恢复，可重新用于知识库问答与 AI 任务。";
    }

    public static String purgeSuccessMessage() {
        return "文档已永久删除，相关数据已清理。";
    }

    public static String formatBytes(Long bytes) {
        if (bytes == null || bytes < 0) {
            return "未知";
        }
        if (bytes < 1024) {
            return bytes + " B";
        }
        if (bytes < 1024 * 1024) {
            return String.format("%.1f KB", bytes / 1024.0);
        }
        return String.format("%.1f MB", bytes / (1024.0 * 1024.0));
    }
}
