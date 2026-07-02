package com.tuoman.ai_task_orchestrator.document.ingestion;

import com.tuoman.ai_task_orchestrator.enums.IngestionEventType;

public final class IngestionEventDisplayTexts {

    private IngestionEventDisplayTexts() {
    }

    public static String displayEventType(IngestionEventType eventType) {
        if (eventType == null) {
            return "未知事件";
        }
        return switch (eventType) {
            case TASK_CREATED -> "文档处理任务已创建";
            case TASK_QUEUED -> "任务已进入处理队列";
            case TASK_STARTED -> "开始处理文档";
            case TEXT_EXTRACTED -> "已读取文档文本";
            case CHUNKING_STARTED -> "开始切分文档内容";
            case CHUNKING_COMPLETED -> "文档切分完成";
            case EMBEDDING_STARTED -> "开始生成文档向量";
            case EMBEDDING_COMPLETED -> "文档向量生成完成";
            case VECTOR_WRITE_STARTED -> "开始写入知识库索引";
            case VECTOR_WRITE_COMPLETED -> "知识库索引写入完成";
            case TASK_COMPLETED -> "文档处理完成";
            case TASK_FAILED -> "文档处理失败";
            case TASK_RETRY_REQUESTED -> "用户请求重新处理";
            case TASK_RETRY_QUEUED -> "重新处理任务已进入队列";
            case DOCUMENT_REINDEX_REQUESTED -> "用户请求重新建立索引";
            case DOCUMENT_REINDEX_QUEUED -> "重新索引任务已进入处理队列";
            case DOCUMENT_REINDEX_STARTED -> "开始重新切分文档内容";
            case DOCUMENT_REINDEX_COMPLETED -> "重新索引完成";
            case DOCUMENT_REINDEX_FAILED -> "重新索引失败";
        };
    }
}
