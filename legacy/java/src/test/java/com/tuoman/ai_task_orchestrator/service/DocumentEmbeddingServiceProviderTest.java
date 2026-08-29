package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.DocumentSearchRequest;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingVectorUtils;
import com.tuoman.ai_task_orchestrator.embedding.MockEmbeddingClient;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEmbeddingEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkEmbeddingRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.transaction.annotation.Transactional;

import java.nio.charset.StandardCharsets;
import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest
@Transactional
class DocumentEmbeddingServiceProviderTest {

    @Autowired
    private DocumentEmbeddingService documentEmbeddingService;

    @Autowired
    private DocumentRepository documentRepository;

    @Autowired
    private DocumentChunkRepository documentChunkRepository;

    @Autowired
    private DocumentChunkEmbeddingRepository documentChunkEmbeddingRepository;

    @Test
    void searchShouldIgnoreEmbeddingsWithDifferentDimension() {
        DocumentEntity document = saveDocument();
        DocumentChunkEntity chunk = saveChunk(document.getId());
        saveMismatchedDimensionEmbedding(document.getId(), chunk.getId());

        DocumentSearchRequest request = new DocumentSearchRequest();
        request.setDocumentId(document.getId());
        request.setQuery("transactional outbox reliable dispatch");
        request.setTopK(5);

        assertThat(documentEmbeddingService.search(request)).isEmpty();
    }

    private DocumentEntity saveDocument() {
        String content = "transactional outbox reliable dispatch";
        DocumentEntity document = new DocumentEntity();
        document.setOriginalFilename("dimension-filter-" + System.nanoTime() + ".md");
        document.setContentType("text/markdown");
        document.setFileSize((long) content.getBytes(StandardCharsets.UTF_8).length);
        document.setStatus(DocumentStatus.CHUNKED);
        document.setChunkCount(1);
        return documentRepository.saveAndFlush(document);
    }

    private DocumentChunkEntity saveChunk(Long documentId) {
        String content = "Transactional Outbox keeps database writes and message dispatch reliable.";
        DocumentChunkEntity chunk = new DocumentChunkEntity();
        chunk.setDocumentId(documentId);
        chunk.setChunkIndex(0);
        chunk.setContent(content);
        chunk.setContentLength(content.length());
        chunk.setChunkStrategy("TEST");
        chunk.setStartOffset(0);
        chunk.setEndOffset(content.length());
        chunk.setHeadingPath("Transactional Outbox");
        return documentChunkRepository.saveAndFlush(chunk);
    }

    private void saveMismatchedDimensionEmbedding(Long documentId, Long chunkId) {
        DocumentChunkEmbeddingEntity embedding = new DocumentChunkEmbeddingEntity();
        embedding.setDocumentId(documentId);
        embedding.setDocumentChunkId(chunkId);
        embedding.setEmbeddingProvider(MockEmbeddingClient.PROVIDER);
        embedding.setEmbeddingModel(MockEmbeddingClient.DEFAULT_MODEL);
        embedding.setVectorDimension(999);
        embedding.setDistanceMetric(MockEmbeddingClient.DISTANCE_METRIC);
        embedding.setEmbeddingVector(EmbeddingVectorUtils.serialize(List.of(0.1, 0.2, 0.3)));
        documentChunkEmbeddingRepository.saveAndFlush(embedding);
    }
}
