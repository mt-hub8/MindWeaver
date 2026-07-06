package com.tuoman.ai_task_orchestrator.batch;

import com.tuoman.ai_task_orchestrator.enums.UploadBatchItemStatus;
import com.tuoman.ai_task_orchestrator.enums.UploadBatchStatus;

public final class UploadBatchDisplayTexts {

    private UploadBatchDisplayTexts() {
    }

    public static String displayBatchStatus(UploadBatchStatus status) {
        if (status == null) {
            return "未知";
        }
        return switch (status) {
            case CREATED -> "已创建";
            case QUEUED -> "排队中";
            case PROCESSING -> "处理中";
            case COMPLETED -> "已完成";
            case PARTIAL_FAILED -> "部分失败";
            case FAILED -> "失败";
            case CANCEL_REQUESTED -> "正在取消";
            case CANCELED -> "已取消";
        };
    }

    public static String displayItemStatus(UploadBatchItemStatus status) {
        if (status == null) {
            return "未知";
        }
        return switch (status) {
            case PENDING -> "待处理";
            case QUEUED -> "排队中";
            case PROCESSING -> "正在处理";
            case COMPLETED -> "已完成";
            case FAILED -> "失败";
            case SKIPPED_DUPLICATE_FILE -> "跳过重复文件";
            case SKIPPED_DUPLICATE_TEXT -> "跳过重复文本";
            case CANCEL_REQUESTED -> "正在取消";
            case CANCELED -> "已取消";
        };
    }
}
