package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.document.extract.DocumentTextExtractorRegistry;
import com.tuoman.ai_task_orchestrator.document.extract.MarkdownTextExtractor;
import com.tuoman.ai_task_orchestrator.document.extract.PdfTextExtractor;
import com.tuoman.ai_task_orchestrator.document.extract.TestDocumentFiles;
import com.tuoman.ai_task_orchestrator.document.extract.TxtTextExtractor;
import com.tuoman.ai_task_orchestrator.document.ingestion.DocumentFileValidator;
import com.tuoman.ai_task_orchestrator.document.ingestion.DocumentIngestionEventRecorder;
import com.tuoman.ai_task_orchestrator.document.ingestion.DocumentIngestionProperties;
import com.tuoman.ai_task_orchestrator.dto.DocumentIngestionSubmitResponse;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentIngestionTaskEntity;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStatus;
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
import static org.mockito.ArgumentMatchers.anyLong;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.ArgumentMatchers.eq;
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

    @Mock
    private DocumentIngestionEventRecorder documentIngestionEventRecorder;

    @Mock
    private DocumentIngestionTaskProgressService documentIngestionTaskProgressService;

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
                documentIngestionMessagePublisher,
                documentIngestionEventRecorder,
                documentIngestionTaskProgressService,
                new com.tuoman.ai_task_orchestrator.document.ingestion.FileHashService()
        );
    }

    @Test
    void submitUploadShouldCreatePendingTaskPublishMessageAndRecordEvents() {
        DocumentEntity document = new DocumentEntity();
        document.setId(10L);
        document.setOriginalFilename("demo.txt");
        when(documentService.createDocumentEntityFromMeta(anyString(), anyString(), anyLong())).thenReturn(document);
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
        assertThat(response.getStatus()).isEqualTo(IngestionTaskStatus.PENDING.name());

        verify(documentIngestionEventRecorder).recordTaskCreated(eq(1001L), eq("demo.txt"), eq(10L));
        verify(documentIngestionEventRecorder).recordTextExtracted(1001L, "demo.txt");
        verify(documentIngestionMessagePublisher).publish(any(DocumentIngestionMessage.class));
        verify(documentIngestionEventRecorder).recordTaskQueued(1001L);
    }

    @Test
    void submitUploadShouldPersistSourceTextForFutureReindex() {
        String extractedText = "cache key content for ingestion";
        DocumentEntity document = new DocumentEntity();
        document.setId(10L);
        document.setOriginalFilename("demo.txt");
        when(documentService.createDocumentEntityFromMeta(anyString(), anyString(), anyLong())).thenReturn(document);
        ArgumentCaptor<DocumentEntity> documentCaptor = ArgumentCaptor.forClass(DocumentEntity.class);
        when(documentRepository.save(documentCaptor.capture())).thenAnswer(invocation -> invocation.getArgument(0));
        when(documentIngestionTaskRepository.save(any())).thenAnswer(invocation -> {
            DocumentIngestionTaskEntity task = invocation.getArgument(0);
            task.setId(1001L);
            return task;
        });

        documentIngestionService.submitUpload(TestDocumentFiles.txtFile("demo.txt", extractedText));

        assertThat(documentCaptor.getValue().getSourceText()).isEqualTo(extractedText);
    }
}
