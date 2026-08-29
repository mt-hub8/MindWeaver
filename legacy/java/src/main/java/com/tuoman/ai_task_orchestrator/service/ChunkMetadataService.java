package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.ChunkMetadataStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentDocType;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentCollectionRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;

import java.util.List;
import java.util.Locale;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

@Service
@RequiredArgsConstructor
public class ChunkMetadataService {

    private static final Pattern VERSION_PATTERN = Pattern.compile("(?i)v(\\d+(?:\\.\\d+)*)");

    private final DocumentCollectionRepository documentCollectionRepository;

    public void applyDocumentMetadata(DocumentEntity document) {
        if (document.getDocType() == null) {
            document.setDocType(guessDocType(document.getOriginalFilename()));
        }
        if (document.getVersion() == null) {
            document.setVersion(extractVersion(document.getOriginalFilename()));
        }
        if (document.getSource() == null) {
            document.setSource(document.getOriginalFilename());
        }
    }

    public void applyChunkMetadata(DocumentEntity document, DocumentChunkEntity chunk) {
        if (document.getDocType() != null) {
            chunk.setDocType(document.getDocType());
        } else {
            chunk.setDocType(guessDocType(document.getOriginalFilename()));
        }
        chunk.setVersion(document.getVersion() != null ? document.getVersion() : extractVersion(document.getOriginalFilename()));
        chunk.setSource(document.getSource() != null ? document.getSource() : document.getOriginalFilename());
        chunk.setTagsJson(document.getTagsJson());
        chunk.setCollectionId(resolvePrimaryCollectionId(document.getId()));
        chunk.setMetadataStatus(mapLifecycleToMetadataStatus(document.getLifecycleStatus()));
        if (chunk.getCharCount() == null && chunk.getContentLength() != null) {
            chunk.setCharCount(chunk.getContentLength());
        }
        if (chunk.getSectionPath() == null) {
            chunk.setSectionPath(chunk.getHeadingPath());
        }
    }

    public void linkChunkRelations(List<DocumentChunkEntity> chunks) {
        linkChunkRelations(chunks, null);
    }

    public void linkChunkRelations(List<DocumentChunkEntity> chunks, List<Integer> parentChunkIndices) {
        for (int i = 0; i < chunks.size(); i++) {
            DocumentChunkEntity chunk = chunks.get(i);
            if (i > 0) {
                chunk.setPreviousChunkId(chunks.get(i - 1).getId());
                chunks.get(i - 1).setNextChunkId(chunk.getId());
            }
            chunk.setChunkUid(documentChunkUid(chunk.getDocumentId(), chunk.getChunkIndex()));
            if (parentChunkIndices != null && i < parentChunkIndices.size()) {
                Integer parentIndex = parentChunkIndices.get(i);
                if (parentIndex != null && parentIndex >= 0 && parentIndex < chunks.size()) {
                    chunk.setParentChunkId(chunks.get(parentIndex).getId());
                }
            }
        }
    }

    public String documentChunkUid(Long documentId, Integer chunkIndex) {
        return documentId + "#" + chunkIndex;
    }

    private Long resolvePrimaryCollectionId(Long documentId) {
        List<Object[]> rows = documentCollectionRepository.findCollectionSummariesByDocumentId(documentId);
        if (rows.isEmpty()) {
            return null;
        }
        Object id = rows.get(0)[0];
        return id instanceof Number number ? number.longValue() : null;
    }

    private DocumentDocType guessDocType(String filename) {
        if (filename == null) {
            return DocumentDocType.OTHER;
        }
        String lower = filename.toLowerCase(Locale.ROOT);
        if (lower.contains("readme")) {
            return DocumentDocType.README;
        }
        if (lower.contains("manual")) {
            return DocumentDocType.MANUAL;
        }
        if (lower.contains("api")) {
            return DocumentDocType.API_DOC;
        }
        if (lower.contains("design")) {
            return DocumentDocType.DESIGN_DOC;
        }
        if (lower.contains("interview") || lower.contains("v")) {
            return DocumentDocType.INTERVIEW_DOC;
        }
        return DocumentDocType.OTHER;
    }

    private String extractVersion(String filename) {
        if (filename == null) {
            return null;
        }
        Matcher matcher = VERSION_PATTERN.matcher(filename);
        if (matcher.find()) {
            return "V" + matcher.group(1).toUpperCase(Locale.ROOT);
        }
        return null;
    }

    private ChunkMetadataStatus mapLifecycleToMetadataStatus(DocumentLifecycleStatus lifecycleStatus) {
        if (lifecycleStatus == null) {
            return ChunkMetadataStatus.ACTIVE;
        }
        return switch (lifecycleStatus) {
            case TRASHED -> ChunkMetadataStatus.TRASHED;
            case PURGED -> ChunkMetadataStatus.PURGED;
            default -> ChunkMetadataStatus.ACTIVE;
        };
    }
}
