package com.tuoman.ai_task_orchestrator.document.ingestion;

import com.tuoman.ai_task_orchestrator.enums.IngestionEventType;

public final class IngestionAnalyticsDisplayTexts {

    private IngestionAnalyticsDisplayTexts() {
    }

    public static String stageDisplayName(String stage) {
        if (stage == null) {
            return "未知阶段";
        }
        return switch (stage) {
            case "CHUNKING" -> "文档切分";
            case "EMBEDDING" -> "生成文档向量";
            case "VECTOR_WRITING" -> "写入知识库索引";
            default -> stage;
        };
    }

    public static String eventTypeToStage(IngestionEventType eventType) {
        return switch (eventType) {
            case CHUNKING_COMPLETED -> "CHUNKING";
            case EMBEDDING_COMPLETED -> "EMBEDDING";
            case VECTOR_WRITE_COMPLETED -> "VECTOR_WRITING";
            default -> null;
        };
    }

    public static String failureReasonDisplayMessage(String errorCode, String errorMessage) {
        if (errorCode == null || errorCode.isBlank()) {
            return failureReasonDisplayMessage("UNKNOWN_ERROR", errorMessage);
        }
        return switch (errorCode) {
            case "UNKNOWN_ERROR" -> "未知错误";
            case "PDF_NO_EXTRACTABLE_TEXT" -> "PDF 没有可提取文字";
            case "EMBEDDING_PROVIDER_ERROR" -> "生成文档向量失败";
            case "VECTOR_STORE_ERROR", "VECTOR_STORE_WRITE_ERROR" -> "写入知识库索引失败";
            case "VALIDATION_ERROR" -> "文档校验失败";
            case "INTERNAL_ERROR" -> "系统内部错误";
            default -> errorMessage == null || errorMessage.isBlank()
                    ? "处理失败，请查看技术详情。"
                    : errorMessage;
        };
    }

    public static String failedTaskDisplayMessage(String errorCode, String errorMessage) {
        String reason = failureReasonDisplayMessage(errorCode, errorMessage);
        return "处理失败：" + reason;
    }
}
