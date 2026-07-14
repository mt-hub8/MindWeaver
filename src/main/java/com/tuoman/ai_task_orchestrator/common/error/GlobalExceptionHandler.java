package com.tuoman.ai_task_orchestrator.common.error;

import com.tuoman.ai_task_orchestrator.embedding.EmbeddingProviderException;
import com.tuoman.ai_task_orchestrator.vectorstore.qdrant.QdrantVectorStoreException;
import lombok.extern.slf4j.Slf4j;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.http.converter.HttpMessageNotReadableException;
import org.springframework.validation.FieldError;
import org.springframework.web.bind.MethodArgumentNotValidException;
import org.springframework.web.bind.annotation.ExceptionHandler;
import org.springframework.web.bind.annotation.RestControllerAdvice;
import org.springframework.web.context.request.ServletWebRequest;
import org.springframework.web.context.request.WebRequest;
import org.springframework.web.server.ResponseStatusException;

import java.time.Instant;
import java.util.NoSuchElementException;
import java.util.UUID;
import java.util.stream.Collectors;

@Slf4j
@RestControllerAdvice
/**
 * V2.2.x production hardening 的统一异常出口。
 *
 * 业务异常、参数校验、外部 embedding/vector store 错误和未捕获异常都会被映射成统一
 * ApiErrorResponse。这样前端、脚本和测试都能依赖稳定的 code/message/traceId，
 * 而不是解析不同异常栈。
 *
 * 关键约束：统一错误响应只改变错误表达方式，不吞掉业务失败，也不把内部异常细节直接暴露给用户。
 */
public class GlobalExceptionHandler {

    private static final String TRACE_ID_HEADER = "X-Request-Id";

    private static final String INTERNAL_ERROR_MESSAGE = "Internal server error";

    @ExceptionHandler(BusinessException.class)
    public ResponseEntity<ApiErrorResponse> handleBusinessException(BusinessException exception, WebRequest request) {
        log.warn("Business exception: code={}, message={}", exception.getErrorCode(), exception.getMessage());
        return buildResponse(exception.getHttpStatus(), exception.getErrorCode(), exception.getMessage(), request);
    }

    @ExceptionHandler(MethodArgumentNotValidException.class)
    public ResponseEntity<ApiErrorResponse> handleMethodArgumentNotValid(
            MethodArgumentNotValidException exception,
            WebRequest request
    ) {
        String message = exception.getBindingResult()
                .getFieldErrors()
                .stream()
                .map(this::formatFieldError)
                .collect(Collectors.joining("; "));
        if (message.isBlank()) {
            message = "Validation failed";
        }
        return buildResponse(HttpStatus.BAD_REQUEST, ErrorCode.VALIDATION_ERROR, message, request);
    }

    @ExceptionHandler(HttpMessageNotReadableException.class)
    public ResponseEntity<ApiErrorResponse> handleHttpMessageNotReadable(
            HttpMessageNotReadableException exception,
            WebRequest request
    ) {
        return buildResponse(HttpStatus.BAD_REQUEST, ErrorCode.INVALID_REQUEST, "Request body is not readable", request);
    }

    @ExceptionHandler(IllegalArgumentException.class)
    public ResponseEntity<ApiErrorResponse> handleIllegalArgumentException(
            IllegalArgumentException exception,
            WebRequest request
    ) {
        String message = exception.getMessage() == null || exception.getMessage().isBlank()
                ? "Invalid request"
                : exception.getMessage();
        return buildResponse(HttpStatus.BAD_REQUEST, ErrorCode.INVALID_REQUEST, message, request);
    }

    @ExceptionHandler(NoSuchElementException.class)
    public ResponseEntity<ApiErrorResponse> handleNoSuchElementException(
            NoSuchElementException exception,
            WebRequest request
    ) {
        String message = exception.getMessage() == null || exception.getMessage().isBlank()
                ? "Resource not found"
                : exception.getMessage();
        return buildResponse(HttpStatus.NOT_FOUND, mapNotFoundErrorCode(message), message, request);
    }

    private ErrorCode mapNotFoundErrorCode(String message) {
        if (message != null && message.toLowerCase().contains("document")) {
            return ErrorCode.DOCUMENT_NOT_FOUND;
        }
        return ErrorCode.TASK_NOT_FOUND;
    }

    @ExceptionHandler(EmbeddingProviderException.class)
    public ResponseEntity<ApiErrorResponse> handleEmbeddingProviderException(
            EmbeddingProviderException exception,
            WebRequest request
    ) {
        log.warn("Embedding provider error: {}", exception.getMessage());
        String message = exception.getMessage() == null || exception.getMessage().isBlank()
                ? "Embedding provider error"
                : exception.getMessage();
        return buildResponse(HttpStatus.INTERNAL_SERVER_ERROR, ErrorCode.EMBEDDING_PROVIDER_ERROR, message, request);
    }

    @ExceptionHandler(QdrantVectorStoreException.class)
    public ResponseEntity<ApiErrorResponse> handleQdrantVectorStoreException(
            QdrantVectorStoreException exception,
            WebRequest request
    ) {
        log.warn("Vector store error: {}", exception.getMessage());
        String message = exception.getMessage() == null || exception.getMessage().isBlank()
                ? "Vector store error"
                : exception.getMessage();
        return buildResponse(HttpStatus.INTERNAL_SERVER_ERROR, ErrorCode.VECTOR_STORE_ERROR, message, request);
    }

    @ExceptionHandler(ResponseStatusException.class)
    public ResponseEntity<ApiErrorResponse> handleResponseStatusException(
            ResponseStatusException exception,
            WebRequest request
    ) {
        HttpStatus status = HttpStatus.resolve(exception.getStatusCode().value());
        if (status == null) {
            status = HttpStatus.INTERNAL_SERVER_ERROR;
        }
        ErrorCode errorCode = mapStatusToErrorCode(status);
        String message = exception.getReason() == null || exception.getReason().isBlank()
                ? status.getReasonPhrase()
                : exception.getReason();
        return buildResponse(status, errorCode, message, request);
    }

    @ExceptionHandler(RuntimeException.class)
    public ResponseEntity<ApiErrorResponse> handleRuntimeException(RuntimeException exception, WebRequest request) {
        log.error("Unhandled runtime exception", exception);
        return buildResponse(
                HttpStatus.INTERNAL_SERVER_ERROR,
                ErrorCode.INTERNAL_ERROR,
                INTERNAL_ERROR_MESSAGE,
                request
        );
    }

    private ResponseEntity<ApiErrorResponse> buildResponse(
            HttpStatus status,
            ErrorCode errorCode,
            String message,
            WebRequest request
    ) {
        // traceId 优先沿用调用方传入的 X-Request-Id。
        // 没有传入时生成新的 UUID，便于把 API 错误和服务端日志关联起来。
        ApiErrorResponse body = new ApiErrorResponse(
                Instant.now().toString(),
                status.value(),
                errorCode.name(),
                message,
                resolvePath(request),
                resolveTraceId(request)
        );
        return ResponseEntity.status(status).body(body);
    }

    private String formatFieldError(FieldError fieldError) {
        if (fieldError.getDefaultMessage() == null || fieldError.getDefaultMessage().isBlank()) {
            return fieldError.getField() + " is invalid";
        }
        return fieldError.getDefaultMessage();
    }

    private String resolvePath(WebRequest request) {
        if (request instanceof ServletWebRequest servletWebRequest) {
            return servletWebRequest.getRequest().getRequestURI();
        }
        return "";
    }

    private String resolveTraceId(WebRequest request) {
        String headerValue = request.getHeader(TRACE_ID_HEADER);
        if (headerValue != null && !headerValue.isBlank()) {
            return headerValue.trim();
        }
        return UUID.randomUUID().toString();
    }

    private ErrorCode mapStatusToErrorCode(HttpStatus status) {
        return switch (status) {
            case NOT_FOUND -> ErrorCode.TASK_NOT_FOUND;
            case CONFLICT -> ErrorCode.INVALID_TASK_STATUS;
            case BAD_REQUEST -> ErrorCode.INVALID_REQUEST;
            default -> ErrorCode.INTERNAL_ERROR;
        };
    }
}
