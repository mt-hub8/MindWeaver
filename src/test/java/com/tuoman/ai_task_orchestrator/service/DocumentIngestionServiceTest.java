package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.document.extract.DocumentTextExtractorRegistry;
import com.tuoman.ai_task_orchestrator.document.extract.TxtTextExtractor;
import com.tuoman.ai_task_orchestrator.document.extract.MarkdownTextExtractor;
import com.tuoman.ai_task_orchestrator.document.extract.PdfTextExtractor;
import com.tuoman.ai_task_orchestrator.document.extract.TestDocumentFiles;
import com.tuoman.ai_task_orchestrator.document.ingestion.DocumentFileValidator;
import com.tuoman.ai_task_orchestrator.document.ingestion.DocumentIngestionProperties;
import com.tuoman.ai_task_orchestrator.dto.DocumentEmbeddingResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentIngestionResponse;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class DocumentIngestionServiceTest {

    @Mock
    private DocumentService documentService;

    @Mock
    private DocumentEmbeddingService documentEmbeddingService;

    @Mock
    private DocumentRepository documentRepository;

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
                documentEmbeddingService,
                documentRepository
        );
    }

    @Test
    void ingestShouldChunkEmbedAndReturnSummary() {
        DocumentEntity document = new DocumentEntity();
        document.setId(10L);
        document.setOriginalFilename("demo.txt");
        when(documentService.createDocumentEntity(any())).thenReturn(document);
        when(documentRepository.save(any())).thenAnswer(invocation -> invocation.getArgument(0));
        when(documentService.chunkAndPersist(eq(document), any())).thenReturn(2);
        when(documentEmbeddingService.embedDocument(10L)).thenReturn(
                new DocumentEmbeddingResponse(10L, "mock", "mock-embedding-v1", 128, "COSINE", 2)
        );

        DocumentIngestionResponse response = documentIngestionService.ingest(
                TestDocumentFiles.txtFile("demo.txt", "cache key content for ingestion")
        );

        assertThat(response.getDocumentId()).isEqualTo(10L);
        assertThat(response.getTitle()).isEqualTo("demo.txt");
        assertThat(response.getStatus()).isEqualTo(DocumentStatus.READY.name());
        assertThat(response.getChunkCount()).isEqualTo(2);
        assertThat(response.getEmbeddingCount()).isEqualTo(2);
        assertThat(response.getVectorWriteCount()).isEqualTo(2);
        verify(documentEmbeddingService).embedDocument(10L);
    }
}
