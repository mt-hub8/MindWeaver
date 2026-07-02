package com.tuoman.ai_task_orchestrator.common.error;

import lombok.Getter;
import org.springframework.http.HttpStatus;

@Getter
public class BusinessException extends RuntimeException {

    private final ErrorCode errorCode;

    private final HttpStatus httpStatus;

    public BusinessException(ErrorCode errorCode, HttpStatus httpStatus, String message) {
        super(message);
        this.errorCode = errorCode;
        this.httpStatus = httpStatus;
    }

    public static BusinessException taskNotFound() {
        return new BusinessException(ErrorCode.TASK_NOT_FOUND, HttpStatus.NOT_FOUND, "任务不存在");
    }

    public static BusinessException documentNotFound() {
        return new BusinessException(ErrorCode.DOCUMENT_NOT_FOUND, HttpStatus.NOT_FOUND, "Document not found");
    }

    public static BusinessException invalidTaskStatus(String message) {
        return new BusinessException(ErrorCode.INVALID_TASK_STATUS, HttpStatus.CONFLICT, message);
    }

    public static BusinessException taskFinalizationRejected(String message) {
        return new BusinessException(ErrorCode.TASK_FINALIZATION_REJECTED, HttpStatus.CONFLICT, message);
    }

    public static BusinessException invalidRequest(String message) {
        return new BusinessException(ErrorCode.INVALID_REQUEST, HttpStatus.BAD_REQUEST, message);
    }

    public static BusinessException validationError(String message) {
        return new BusinessException(ErrorCode.VALIDATION_ERROR, HttpStatus.BAD_REQUEST, message);
    }

    public static BusinessException vectorStoreError(String message) {
        return new BusinessException(ErrorCode.VECTOR_STORE_ERROR, HttpStatus.INTERNAL_SERVER_ERROR, message);
    }

    public static BusinessException llmProviderError(String message) {
        return new BusinessException(ErrorCode.LLM_PROVIDER_ERROR, HttpStatus.INTERNAL_SERVER_ERROR, message);
    }

    public static BusinessException retrievalEvaluationError(String message) {
        return new BusinessException(ErrorCode.RETRIEVAL_EVALUATION_ERROR, HttpStatus.BAD_REQUEST, message);
    }

    public static BusinessException ingestionTaskNotFound() {
        return new BusinessException(ErrorCode.INGESTION_TASK_NOT_FOUND, HttpStatus.NOT_FOUND, "文档摄入任务不存在");
    }

    public static BusinessException ingestionRetryNotAllowed(String message) {
        return new BusinessException(ErrorCode.INGESTION_RETRY_NOT_ALLOWED, HttpStatus.CONFLICT, message);
    }

    public static BusinessException ingestionMaxRetryExceeded() {
        return new BusinessException(
                ErrorCode.INGESTION_MAX_RETRY_EXCEEDED,
                HttpStatus.CONFLICT,
                "已达到最大重试次数，请检查失败原因后重新上传文档"
        );
    }

    public static BusinessException documentDeletedCannotReindex() {
        return new BusinessException(
                ErrorCode.DOCUMENT_DELETED_CANNOT_REINDEX,
                HttpStatus.CONFLICT,
                "当前文档已删除，不能重新索引"
        );
    }

    public static BusinessException documentSourceTextMissing() {
        return new BusinessException(
                ErrorCode.DOCUMENT_SOURCE_TEXT_MISSING,
                HttpStatus.BAD_REQUEST,
                "当前文档缺少原始文本，无法重新建立索引。请重新上传文档。"
        );
    }

    public static BusinessException documentReindexAlreadyRunning() {
        return new BusinessException(
                ErrorCode.DOCUMENT_REINDEX_ALREADY_RUNNING,
                HttpStatus.CONFLICT,
                "该文档已有重新索引任务正在处理。"
        );
    }

    public static BusinessException internalError(String message) {
        return new BusinessException(ErrorCode.INTERNAL_ERROR, HttpStatus.INTERNAL_SERVER_ERROR, message);
    }

    public static BusinessException collectionNotFound() {
        return new BusinessException(ErrorCode.COLLECTION_NOT_FOUND, HttpStatus.NOT_FOUND, "知识库分组不存在");
    }

    public static BusinessException collectionNameRequired() {
        return new BusinessException(ErrorCode.COLLECTION_NAME_REQUIRED, HttpStatus.BAD_REQUEST, "分组名称不能为空");
    }

    public static BusinessException collectionNameDuplicated() {
        return new BusinessException(ErrorCode.COLLECTION_NAME_DUPLICATED, HttpStatus.CONFLICT, "分组名称已存在");
    }

    public static BusinessException documentAlreadyInCollection() {
        return new BusinessException(
                ErrorCode.DOCUMENT_ALREADY_IN_COLLECTION,
                HttpStatus.CONFLICT,
                "该文档已加入此分组"
        );
    }

    public static BusinessException documentNotInCollection() {
        return new BusinessException(
                ErrorCode.DOCUMENT_NOT_IN_COLLECTION,
                HttpStatus.NOT_FOUND,
                "该文档未加入此分组"
        );
    }

    public static BusinessException agentTaskNotFound() {
        return new BusinessException(ErrorCode.AGENT_TASK_NOT_FOUND, HttpStatus.NOT_FOUND, "AI 任务不存在");
    }

    public static BusinessException agentTaskCollectionNotFound() {
        return new BusinessException(
                ErrorCode.AGENT_TASK_COLLECTION_NOT_FOUND,
                HttpStatus.NOT_FOUND,
                "任务绑定的知识库分组不存在"
        );
    }

    public static BusinessException agentTaskRetrievalFailed(String message) {
        return new BusinessException(
                ErrorCode.AGENT_TASK_RETRIEVAL_FAILED,
                HttpStatus.INTERNAL_SERVER_ERROR,
                message
        );
    }

    public static BusinessException agentTaskLlmFailed(String message) {
        return new BusinessException(ErrorCode.AGENT_TASK_LLM_FAILED, HttpStatus.INTERNAL_SERVER_ERROR, message);
    }

    public static BusinessException aiRuntimeUnavailable(String message) {
        return new BusinessException(ErrorCode.AI_RUNTIME_UNAVAILABLE, HttpStatus.SERVICE_UNAVAILABLE, message);
    }

    public static BusinessException aiRuntimeTimeout(String message) {
        return new BusinessException(ErrorCode.AI_RUNTIME_TIMEOUT, HttpStatus.GATEWAY_TIMEOUT, message);
    }

    public static BusinessException agentTaskExecutionFailed(String message) {
        return new BusinessException(ErrorCode.AGENT_TASK_EXECUTION_FAILED, HttpStatus.INTERNAL_SERVER_ERROR, message);
    }
}
