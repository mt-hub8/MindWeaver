package com.tuoman.ai_task_orchestrator.retrieval;

import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.ChunkMetadataStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentCollectionRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;

import java.util.HashSet;
import java.util.List;
import java.util.Locale;
import java.util.Set;
import java.util.stream.Collectors;

@Service
@RequiredArgsConstructor
public class RetrievalFilterService {

    private final DocumentRepository documentRepository;

    private final DocumentCollectionRepository documentCollectionRepository;

    private final DocumentChunkRepository documentChunkRepository;

    public FilterResolution resolve(RetrievalFilter filter) {
        if (filter == null) {
            filter = RetrievalFilter.empty();
        }
        Set<Long> allowedDocuments = resolveAllowedDocumentIds(filter);
        boolean applicationSide = true;
        return new FilterResolution(filter, allowedDocuments, applicationSide);
    }

    public boolean matchesChunk(DocumentChunkEntity chunk, DocumentEntity document, RetrievalFilter filter) {
        if (chunk == null) {
            return false;
        }
        if (!filter.isIncludeTrashed()) {
            ChunkMetadataStatus status = chunk.getMetadataStatus();
            if (status == ChunkMetadataStatus.TRASHED || status == ChunkMetadataStatus.PURGED) {
                return false;
            }
            if (document != null && document.getLifecycleStatus() != null
                    && document.getLifecycleStatus() != DocumentLifecycleStatus.ACTIVE) {
                return false;
            }
        }
        if (!filter.isIncludeDeprecated()) {
            if (chunk.getMetadataStatus() == ChunkMetadataStatus.DEPRECATED) {
                return false;
            }
        }
        if (!filter.isIncludeDraft() && chunk.getMetadataStatus() == ChunkMetadataStatus.DRAFT) {
            return false;
        }
        if (filter.getCollectionId() != null && chunk.getCollectionId() != null
                && !filter.getCollectionId().equals(chunk.getCollectionId())) {
            return false;
        }
        if (filter.getVersion() != null && chunk.getVersion() != null
                && !filter.getVersion().equalsIgnoreCase(chunk.getVersion())) {
            return false;
        }
        if (filter.getDocType() != null && chunk.getDocType() != null
                && filter.getDocType() != chunk.getDocType()) {
            return false;
        }
        if (filter.getSource() != null && chunk.getSource() != null
                && !chunk.getSource().toLowerCase(Locale.ROOT).contains(filter.getSource().toLowerCase(Locale.ROOT))) {
            return false;
        }
        return true;
    }

    public Set<Long> resolveAllowedDocumentIds(RetrievalFilter filter) {
        Set<Long> ids = new HashSet<>();
        if (filter.getScopedDocumentIds() != null && !filter.getScopedDocumentIds().isEmpty()) {
            ids.addAll(filter.getScopedDocumentIds());
        } else if (filter.getCollectionId() != null) {
            ids.addAll(documentCollectionRepository.findAskableDocumentIdsByCollectionId(filter.getCollectionId()));
        } else {
            documentRepository.findAll().forEach(doc -> {
                if (doc.getLifecycleStatus() == DocumentLifecycleStatus.ACTIVE) {
                    ids.add(doc.getId());
                }
            });
        }
        return ids.stream()
                .filter(id -> documentRepository.findById(id)
                        .map(doc -> filter.isIncludeTrashed() || doc.getLifecycleStatus() == DocumentLifecycleStatus.ACTIVE)
                        .orElse(false))
                .collect(Collectors.toSet());
    }

    public List<DocumentChunkEntity> loadFilteredChunks(Set<Long> documentIds) {
        return documentIds.stream()
                .flatMap(id -> documentChunkRepository.findByDocumentIdOrderByChunkIndexAsc(id).stream())
                .filter(chunk -> chunk.getChunkStatus() != null)
                .toList();
    }

    public record FilterResolution(RetrievalFilter filter, Set<Long> allowedDocumentIds, boolean applicationSideFilter) {
    }
}
