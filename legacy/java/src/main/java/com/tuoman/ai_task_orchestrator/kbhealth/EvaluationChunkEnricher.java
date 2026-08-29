package com.tuoman.ai_task_orchestrator.kbhealth;

import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.repository.DocumentCollectionRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Component;

import java.util.HashMap;
import java.util.HashSet;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Set;

@Component
@RequiredArgsConstructor
public class EvaluationChunkEnricher {

    private final DocumentRepository documentRepository;

    private final DocumentCollectionRepository documentCollectionRepository;

    public EvaluationRetrievedChunk enrich(
            EvaluationRetrievedChunk chunk,
            Long expectedCollectionId,
            String expectedVersion,
            Set<Long> expectedChunkIds,
            Set<Long> expectedDocIds,
            Set<Long> negativeDocIds
    ) {
        DocumentEntity document = chunk.getDocumentId() == null
                ? null
                : documentRepository.findById(chunk.getDocumentId()).orElse(null);

        Long primaryCollectionId = resolvePrimaryCollection(chunk.getDocumentId());
        String versionHint = resolveVersionHint(document);
        String docType = document != null && document.getOriginalFilename() != null
                ? guessDocType(document.getOriginalFilename())
                : null;

        boolean wrongCollection = expectedCollectionId != null
                && primaryCollectionId != null
                && !expectedCollectionId.equals(primaryCollectionId);
        boolean wrongVersion = expectedVersion != null
                && !expectedVersion.isBlank()
                && versionHint != null
                && !versionHint.equalsIgnoreCase(expectedVersion);

        boolean expected = expectedChunkIds.contains(chunk.getChunkId())
                || expectedDocIds.contains(chunk.getDocumentId());
        boolean negative = negativeDocIds.contains(chunk.getDocumentId());

        Map<String, Object> metadata = chunk.getMetadataJson() == null
                ? new HashMap<>()
                : new HashMap<>(chunk.getMetadataJson());
        metadata.put("versionHint", versionHint);
        metadata.put("primaryCollectionId", primaryCollectionId);

        return new EvaluationRetrievedChunk(
                chunk.getChunkId(),
                chunk.getDocumentId(),
                primaryCollectionId,
                docType,
                versionHint,
                chunk.getSource(),
                chunk.getSectionPath(),
                chunk.getTextSnippet(),
                chunk.getScore(),
                chunk.getRank(),
                chunk.getRetrievalSource(),
                metadata,
                expected,
                negative,
                wrongCollection,
                wrongVersion
        );
    }

    public Set<Long> resolveScopedDocumentIds(Long collectionId) {
        if (collectionId == null) {
            return Set.of();
        }
        return new HashSet<>(documentCollectionRepository.findAskableDocumentIdsByCollectionId(collectionId));
    }

    private Long resolvePrimaryCollection(Long documentId) {
        if (documentId == null) {
            return null;
        }
        List<Object[]> rows = documentCollectionRepository.findCollectionSummariesByDocumentId(documentId);
        if (rows.isEmpty()) {
            return null;
        }
        Object id = rows.get(0)[0];
        if (id instanceof Number number) {
            return number.longValue();
        }
        return null;
    }

    private String resolveVersionHint(DocumentEntity document) {
        if (document == null || document.getOriginalFilename() == null) {
            return document == null ? null : "gen-" + document.getCurrentGeneration();
        }
        String filename = document.getOriginalFilename();
        java.util.regex.Matcher matcher = java.util.regex.Pattern
                .compile("(?i)v(\\d+(?:\\.\\d+)*)")
                .matcher(filename);
        if (matcher.find()) {
            return "V" + matcher.group(1).toUpperCase(Locale.ROOT);
        }
        return "gen-" + document.getCurrentGeneration();
    }

    private String guessDocType(String filename) {
        String lower = filename.toLowerCase(Locale.ROOT);
        if (lower.endsWith(".md")) {
            return "markdown";
        }
        if (lower.endsWith(".pdf")) {
            return "pdf";
        }
        if (lower.endsWith(".txt")) {
            return "text";
        }
        return "unknown";
    }
}
