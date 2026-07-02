package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.document.extract.DocumentTextExtractorRegistry;
import com.tuoman.ai_task_orchestrator.document.extract.MarkdownTextExtractor;
import com.tuoman.ai_task_orchestrator.document.extract.PdfTextExtractor;
import com.tuoman.ai_task_orchestrator.document.extract.TestDocumentFiles;
import com.tuoman.ai_task_orchestrator.document.extract.TxtTextExtractor;
import com.tuoman.ai_task_orchestrator.document.ingestion.DocumentFileValidator;
import com.tuoman.ai_task_orchestrator.document.ingestion.DocumentIngestionProperties;
import com.tuoman.ai_task_orchestrator.dto.DocumentIngestionSubmitResponse;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentIngestionTaskEntity;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStatus;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStep;
import com.tuoman.ai_task_orchestrator.mq.DocumentIngestionMessage;
import com.tuoman.ai_task_orchestrator.mq.DocumentIngestionMessagePublisher;
import com.tuoman.ai_task_orchestrator.repository.DocumentIngestionTaskRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.ArgumentCaptor;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class DocumentIngestionServiceTest {

    @Mock
    private DocumentService documentService;

    @Mock
    private DocumentRepository documentRepository;

    @Mock
    private DocumentIngestionTaskRepository documentIngestionTaskRepository;

    @Mock
    private DocumentIngestionMessagePublisher documentIngestionMessagePublisher;

    private DocumentIngestionService documentIngestionService;

    @BeforeEach
    void setUp() {
        DocumentIngestionProperties properties = new DocumentIngestionProperties();
        properties.setMaxFileSizeBytes(2 * 1024 * 1024);
        DocumentFileValidator validator = new DocumentFileValidator(properties);
        DocumentTextExtractorRegistry registry = new DocumentTextExtractorRegistry(
                new TxtTextExtractor(),
                new MarkdownTextExtractor(new TxtTextExtractor()),
                new PdfTextExtractor()
        );
        documentIngestionService = new DocumentIngestionService(
                validator,
                registry,
                documentService,
                documentRepository,
                documentIngestionTaskRepository,
                documentIngestionMessagePublisher
        );
    }

    @Test
    void submitUploadShouldCreatePendingTaskAndPublishMessage() {
        DocumentEntity document = new DocumentEntity();
        document.setId(10L);
        document.setOriginalFilename("demo.txt");
        when(documentService.createDocumentEntity(any())).thenReturn(document);
        when(documentRepository.save(any())).thenAnswer(invocation -> invocation.getArgument(0));
        when(documentIngestionTaskRepository.save(any())).thenAnswer(invocation -> {
            DocumentIngestionTaskEntity task = invocation.getArgument(0);
            task.setId(1001L);
            return task;
        });

        DocumentIngestionSubmitResponse response = documentIngestionService.submitUpload(
                TestDocumentFiles.txtFile("demo.txt", "cache key content for ingestion")
        );

        assertThat(response.getTaskId()).isEqualTo(1001L);
        assertThat(response.getDocumentId()).isEqualTo(10L);
        assertThat(response.getFilename()).isEqualTo("demo.txt");
        assertThat(response.getStatus()).isEqualTo(IngestionTaskStatus.PENDING.name());
        assertThat(response.getDisplayStatus()).isEqualTo("待处理");
        assertThat(response.getDisplayMessage()).contains("排队处理");

        ArgumentCaptor<DocumentIngestionTaskEntity> taskCaptor = ArgumentCaptor.forClass(DocumentIngestionTaskEntity.class);
        verify(documentIngestionTaskRepository).save(taskCaptor.capture());
        assertThat(taskCaptor.getValue().getStatus()).isEqualTo(IngestionTaskStatus.PENDING);
        assertThat(taskCaptor.getValue().getStep()).isEqualTo(IngestionTaskStep.TEXT_EXTRACTED);

        ArgumentCaptor<DocumentIngestionMessage> messageCaptor = ArgumentCaptor.forClass(DocumentIngestionMessage.class);
        verify(documentIngestionMessagePublisher).publish(messageCaptor.capture());
        assertThat(messageCaptor.getValue().getTaskId()).isEqualTo(1001L);
        assertThat(messageCaptor.getValue().getDocumentId()).isEqualTo(10L);
    }
}
