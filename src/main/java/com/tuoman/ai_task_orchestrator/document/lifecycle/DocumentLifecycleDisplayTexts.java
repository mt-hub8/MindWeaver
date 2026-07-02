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
            case DELETED -> "已删除";
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
        if (lifecycleStatus == DocumentLifecycleStatus.DELETED) {
            return "该文档已删除，不会再用于知识库问答。";
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
        return "删除成功：该文档不会再用于知识库问答。";
    }

    public static String deleteAlreadyDeletedMessage() {
        return "该文档已删除，不会再用于知识库问答。";
    }
}
