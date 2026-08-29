package com.tuoman.ai_task_orchestrator.common.error;

import com.tuoman.ai_task_orchestrator.embedding.EmbeddingProviderException;
import com.tuoman.ai_task_orchestrator.vectorstore.qdrant.QdrantVectorStoreException;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.http.converter.HttpMessageNotReadableException;
import org.springframework.mock.web.MockHttpServletRequest;
import org.springframework.core.MethodParameter;
import org.springframework.validation.BeanPropertyBindingResult;
import org.springframework.validation.FieldError;
import org.springframework.web.bind.MethodArgumentNotValidException;
import org.springframework.web.context.request.ServletWebRequest;

import java.util.NoSuchElementException;

import static org.assertj.core.api.Assertions.assertThat;

class GlobalExceptionHandlerTest {

    private GlobalExceptionHandler handler;

    private MockHttpServletRequest servletRequest;

    private ServletWebRequest webRequest;

    @BeforeEach
    void setUp() {
        handler = new GlobalExceptionHandler();
        servletRequest = new MockHttpServletRequest();
        servletRequest.setRequestURI("/tasks/1");
        webRequest = new ServletWebRequest(servletRequest);
    }

    @Test
    void businessExceptionShouldReturnMappedStatusCodeAndFields() {
        BusinessException exception = BusinessException.taskNotFound();

        ResponseEntity<ApiErrorResponse> response = handler.handleBusinessException(exception, webRequest);

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.NOT_FOUND);
        ApiErrorResponse body = response.getBody();
        assertThat(body).isNotNull();
        assertThat(body.getStatus()).isEqualTo(404);
        assertThat(body.getCode()).isEqualTo("TASK_NOT_FOUND");
        assertThat(body.getMessage()).isEqualTo("任务不存在");
        assertThat(body.getPath()).isEqualTo("/tasks/1");
        assertThat(body.getTimestamp()).isNotBlank();
        assertThat(body.getTraceId()).isNotBlank();
    }

    @Test
    void llmProviderBusinessExceptionShouldReturnMappedErrorCode() {
        BusinessException exception = BusinessException.llmProviderError("llm failed");

        ResponseEntity<ApiErrorResponse> response = handler.handleBusinessException(exception, webRequest);

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.INTERNAL_SERVER_ERROR);
        assertThat(response.getBody()).isNotNull();
        assertThat(response.getBody().getCode()).isEqualTo("LLM_PROVIDER_ERROR");
    }

    @Test
    void illegalArgumentExceptionShouldReturnInvalidRequest() {
        ResponseEntity<ApiErrorResponse> response = handler.handleIllegalArgumentException(
                new IllegalArgumentException("bad input"),
                webRequest
        );

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.BAD_REQUEST);
        assertThat(response.getBody()).isNotNull();
        assertThat(response.getBody().getCode()).isEqualTo("INVALID_REQUEST");
        assertThat(response.getBody().getMessage()).isEqualTo("bad input");
    }

    @Test
    void methodArgumentNotValidExceptionShouldReturnValidationError() throws NoSuchMethodException {
        BeanPropertyBindingResult bindingResult = new BeanPropertyBindingResult(new Object(), "request");
        bindingResult.addError(new FieldError("request", "prompt", "prompt不能为空"));
        MethodParameter parameter = new MethodParameter(
                ValidationTarget.class.getDeclaredMethod("setPrompt", String.class),
                0
        );
        MethodArgumentNotValidException exception = new MethodArgumentNotValidException(parameter, bindingResult);

        ResponseEntity<ApiErrorResponse> response = handler.handleMethodArgumentNotValid(exception, webRequest);

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.BAD_REQUEST);
        assertThat(response.getBody()).isNotNull();
        assertThat(response.getBody().getCode()).isEqualTo("VALIDATION_ERROR");
        assertThat(response.getBody().getMessage()).isEqualTo("prompt不能为空");
    }

    @Test
    void httpMessageNotReadableExceptionShouldReturnInvalidRequest() {
        ResponseEntity<ApiErrorResponse> response = handler.handleHttpMessageNotReadable(
                new HttpMessageNotReadableException("invalid json"),
                webRequest
        );

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.BAD_REQUEST);
        assertThat(response.getBody()).isNotNull();
        assertThat(response.getBody().getCode()).isEqualTo("INVALID_REQUEST");
        assertThat(response.getBody().getMessage()).isEqualTo("Request body is not readable");
    }

    @Test
    void noSuchElementExceptionShouldReturnTaskNotFoundWhenMessageMatchesTask() {
        ResponseEntity<ApiErrorResponse> response = handler.handleNoSuchElementException(
                new NoSuchElementException("Task not found"),
                webRequest
        );

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.NOT_FOUND);
        assertThat(response.getBody()).isNotNull();
        assertThat(response.getBody().getCode()).isEqualTo("TASK_NOT_FOUND");
    }

    @Test
    void noSuchElementExceptionShouldReturnDocumentNotFoundWhenMessageMatchesDocument() {
        servletRequest.setRequestURI("/documents/9");
        ResponseEntity<ApiErrorResponse> response = handler.handleNoSuchElementException(
                new NoSuchElementException("Document not found"),
                webRequest
        );

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.NOT_FOUND);
        assertThat(response.getBody()).isNotNull();
        assertThat(response.getBody().getCode()).isEqualTo("DOCUMENT_NOT_FOUND");
    }

    @Test
    void runtimeExceptionShouldReturnInternalErrorWithoutStackDetails() {
        ResponseEntity<ApiErrorResponse> response = handler.handleRuntimeException(
                new RuntimeException("database exploded"),
                webRequest
        );

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.INTERNAL_SERVER_ERROR);
        assertThat(response.getBody()).isNotNull();
        assertThat(response.getBody().getCode()).isEqualTo("INTERNAL_ERROR");
        assertThat(response.getBody().getMessage()).isEqualTo("Internal server error");
        assertThat(response.getBody().getMessage()).doesNotContain("database exploded");
    }

    @Test
    void embeddingProviderExceptionShouldReturnEmbeddingProviderError() {
        ResponseEntity<ApiErrorResponse> response = handler.handleEmbeddingProviderException(
                new EmbeddingProviderException("embedding failed"),
                webRequest
        );

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.INTERNAL_SERVER_ERROR);
        assertThat(response.getBody()).isNotNull();
        assertThat(response.getBody().getCode()).isEqualTo("EMBEDDING_PROVIDER_ERROR");
        assertThat(response.getBody().getMessage()).isEqualTo("embedding failed");
    }

    @Test
    void qdrantVectorStoreExceptionShouldReturnVectorStoreError() {
        ResponseEntity<ApiErrorResponse> response = handler.handleQdrantVectorStoreException(
                new QdrantVectorStoreException("qdrant failed"),
                webRequest
        );

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.INTERNAL_SERVER_ERROR);
        assertThat(response.getBody()).isNotNull();
        assertThat(response.getBody().getCode()).isEqualTo("VECTOR_STORE_ERROR");
        assertThat(response.getBody().getMessage()).isEqualTo("qdrant failed");
    }

    @Test
    void traceIdShouldUseRequestHeaderWhenPresent() {
        servletRequest.addHeader("X-Request-Id", "trace-from-client");

        ResponseEntity<ApiErrorResponse> response = handler.handleBusinessException(
                BusinessException.taskNotFound(),
                webRequest
        );

        assertThat(response.getBody()).isNotNull();
        assertThat(response.getBody().getTraceId()).isEqualTo("trace-from-client");
    }

    @Test
    void traceIdShouldBeGeneratedWhenRequestHeaderMissing() {
        ResponseEntity<ApiErrorResponse> response = handler.handleBusinessException(
                BusinessException.taskNotFound(),
                webRequest
        );

        assertThat(response.getBody()).isNotNull();
        assertThat(response.getBody().getTraceId()).isNotBlank();
        assertThat(response.getBody().getTraceId()).isNotEqualTo("trace-from-client");
    }

    static class ValidationTarget {

        void setPrompt(String prompt) {
        }
    }
}
