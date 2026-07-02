package com.tuoman.ai_task_orchestrator.document.ingestion;

import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStatus;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStep;

public final class IngestionDisplayTexts {

    private IngestionDisplayTexts() {
    }

    public static String displayStatus(IngestionTaskStatus status) {
        if (status == null) {
            return "未知状态";
        }
        return switch (status) {
            case PENDING -> "待处理";
            case PROCESSING -> "处理中";
            case COMPLETED -> "已完成";
            case FAILED -> "失败";
        };
    }

    public static String displayStep(IngestionTaskStep step) {
        if (step == null) {
            return "未知步骤";
        }
        return switch (step) {
            case UPLOADED -> "文档已提交";
            case TEXT_EXTRACTED -> "已读取文档文本";
            case CHUNKING -> "正在切分文档内容";
            case EMBEDDING -> "正在生成文档向量";
            case VECTOR_WRITING -> "正在写入知识库索引";
            case COMPLETED -> "处理完成，可以开始提问";
            case FAILED -> "处理失败，请查看原因";
        };
    }

    public static String displayMessage(IngestionTaskStatus status, IngestionTaskStep step) {
        if (status == IngestionTaskStatus.PENDING) {
            return "文档已提交，正在排队处理。";
        }
        if (status == IngestionTaskStatus.COMPLETED) {
            return "知识库索引已建立，现在可以前往「知识库问答」页面提问。";
        }
        if (status == IngestionTaskStatus.FAILED) {
            return "文档处理失败，请查看失败原因，或点击「重新处理」。";
        }
        if (step == IngestionTaskStep.CHUNKING) {
            return "系统正在将文档切分为可检索的文档片段（Chunk）。";
        }
        if (step == IngestionTaskStep.EMBEDDING) {
            return "系统正在为文档片段生成向量，请稍候。";
        }
        if (step == IngestionTaskStep.VECTOR_WRITING) {
            return "系统正在将向量写入知识库索引（Vector Index）。";
        }
        return "系统正在处理文档，请稍候。";
    }
}
