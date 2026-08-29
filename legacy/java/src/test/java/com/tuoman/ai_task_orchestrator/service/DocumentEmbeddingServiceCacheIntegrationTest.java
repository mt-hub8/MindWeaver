package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.embedding.ChunkHashService;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingProvider;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingRequest;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingResponse;
import com.tuoman.ai_task_orchestrator.embedding.MockEmbeddingClient;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEmbeddingEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.entity.EmbeddingCacheEntity;
import com.tuoman.ai_task_orchestrator.enums.ChunkStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkEmbeddingRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.repository.EmbeddingCacheRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.transaction.annotation.Transactional;

import java.nio.charset.StandardCharsets;
import java.util.List;
import java.util.concurrent.atomic.AtomicInteger;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.atLeastOnce;
import static org.mockito.Mockito.times;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@SpringBootTest
@Transactional
class DocumentEmbeddingServiceCacheIntegrationTest {

    private static final String SHARED_CHUNK_CONTENT =
            "Transactional Outbox keeps database writes and message dispatch reliable.";

    @Autowired
    private DocumentEmbeddingService documentEmbeddingService;

    @Autowired
    private DocumentRepository documentRepository;

    @Autowired
    private DocumentChunkRepository documentChunkRepository;

    @Autowired
    private DocumentChunkEmbeddingRepository documentChunkEmbeddingRepository;

    @Autowired
    private EmbeddingCacheRepository embeddingCacheRepository;

    @Autowired
    private ChunkHashService chunkHashService;

    @MockitoBean
    private EmbeddingProvider embeddingProvider;

    private final AtomicInteger embedInvocationCount = new AtomicInteger();

    @BeforeEach
    void setUpProvider() {
        embedInvocationCount.set(0);
        when(embeddingProvider.provider()).thenReturn(MockEmbeddingClient.PROVIDER);
        when(embeddingProvider.model()).thenReturn(MockEmbeddingClient.DEFAULT_MODEL);
        when(embeddingProvider.dimension()).thenReturn(MockEmbeddingClient.DIMENSION);
        when(embeddingProvider.embed(any(EmbeddingRequest.class))).thenAnswer(invocation -> {
            embedInvocationCount.incrementAndGet();
            EmbeddingRequest request = invocation.getArgument(0);
            MockEmbeddingClient delegate = new MockEmbeddingClient();
            EmbeddingRequest delegateRequest = new EmbeddingRequest();
            delegateRequest.setText(request.getText());
            delegateRequest.setModel(request.getModel());
            return delegate.embed(delegateRequest);
        });
    }

    @Test
    void shouldUseCacheOnSecondDocumentWithSameChunkContent() {
        DocumentEntity firstDocument = saveDocument("first-doc");
        saveChunk(firstDocument.getId(), SHARED_CHUNK_CONTENT);
        DocumentEntity secondDocument = saveDocument("second-doc");
        saveChunk(secondDocument.getId(), SHARED_CHUNK_CONTENT);

        documentEmbeddingService.embedDocument(firstDocument.getId());
        int callsAfterFirstDocument = embedInvocationCount.get();
        assertThat(callsAfterFirstDocument).isEqualTo(1);

        documentEmbeddingService.embedDocument(secondDocument.getId());
        assertThat(embedInvocationCount.get()).isEqualTo(callsAfterFirstDocument);

        String chunkHash = chunkHashService.hash(SHARED_CHUNK_CONTENT);
        assertThat(embeddingCacheRepository.findByChunkHashAndProviderAndModelAndDimension(
                chunkHash,
                MockEmbeddingClient.PROVIDER,
                MockEmbeddingClient.DEFAULT_MODEL,
                MockEmbeddingClient.DIMENSION
        )).isPresent();

        List<DocumentChunkEmbeddingEntity> embeddings = documentChunkEmbeddingRepository
                .findByDocumentIdAndEmbeddingProviderAndEmbeddingModel(
                        secondDocument.getId(),
                        MockEmbeddingClient.PROVIDER,
                        MockEmbeddingClient.DEFAULT_MODEL
                );
        assertThat(embeddings).hasSize(1);
    }

    @Test
    void shouldStillUpsertVectorStoreOnCacheHit() {
        DocumentEntity firstDocument = saveDocument("upsert-first");
        DocumentChunkEntity firstChunk = saveChunk(firstDocument.getId(), SHARED_CHUNK_CONTENT);
        documentEmbeddingService.embedDocument(firstDocument.getId());

        DocumentEntity secondDocument = saveDocument("upsert-second");
        DocumentChunkEntity secondChunk = saveChunk(secondDocument.getId(), SHARED_CHUNK_CONTENT);
        documentEmbeddingService.embedDocument(secondDocument.getId());

        verify(embeddingProvider, atLeastOnce()).embed(any(EmbeddingRequest.class));
        verify(embeddingProvider, times(1)).embed(any(EmbeddingRequest.class));

        List<DocumentChunkEmbeddingEntity> secondDocumentEmbeddings = documentChunkEmbeddingRepository
                .findByDocumentIdAndEmbeddingProviderAndEmbeddingModel(
                        secondDocument.getId(),
                        MockEmbeddingClient.PROVIDER,
                        MockEmbeddingClient.DEFAULT_MODEL
                );
        assertThat(secondDocumentEmbeddings).hasSize(1);
        assertThat(secondDocumentEmbeddings.getFirst().getDocumentChunkId()).isEqualTo(secondChunk.getId());
        assertThat(firstChunk.getId()).isNotEqualTo(secondChunk.getId());
    }

    private DocumentEntity saveDocument(String label) {
        DocumentEntity document = new DocumentEntity();
        document.setOriginalFilename(label + "-" + System.nanoTime() + ".md");
        document.setContentType("text/markdown");
        document.setFileSize((long) label.getBytes(StandardCharsets.UTF_8).length);
        document.setStatus(DocumentStatus.CHUNKED);
        document.setChunkCount(1);
        document.setCurrentGeneration(1);
        document.setReindexCount(0);
        return documentRepository.saveAndFlush(document);
    }

    private DocumentChunkEntity saveChunk(Long documentId, String content) {
        DocumentChunkEntity chunk = new DocumentChunkEntity();
        chunk.setDocumentId(documentId);
        chunk.setChunkIndex(0);
        chunk.setContent(content);
        chunk.setContentLength(content.length());
        chunk.setChunkStrategy("TEST");
        chunk.setStartOffset(0);
        chunk.setEndOffset(content.length());
        chunk.setHeadingPath("Test");
        chunk.setChunkStatus(ChunkStatus.ACTIVE);
        chunk.setGeneration(1);
        return documentChunkRepository.saveAndFlush(chunk);
    }
}
