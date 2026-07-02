package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.DocumentSearchResultResponse;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.hybrid.LexicalCandidate;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.HashSet;
import java.util.List;
import java.util.Set;

@Service
@RequiredArgsConstructor
public class DocumentLifecycleFilterService {

    private final DocumentRepository documentRepository;

    private final DocumentChunkRepository documentChunkRepository;

    @Transactional(readOnly = true)
    public Set<Long> findDeletedDocumentIds() {
        return new HashSet<>(documentRepository.findIdsByLifecycleStatus(DocumentLifecycleStatus.DELETED));
    }

    @Transactional(readOnly = true)
    public Set<Long> findRetrievableChunkIds() {
        return new HashSet<>(documentChunkRepository.findRetrievableChunkIds());
    }

    public boolean isDeleted(Long documentId, Set<Long> deletedDocumentIds) {
        return documentId != null && deletedDocumentIds.contains(documentId);
    }

    public boolean isRetrievableChunk(Long chunkId, Set<Long> retrievableChunkIds) {
        return chunkId != null && retrievableChunkIds.contains(chunkId);
    }

    public List<DocumentSearchResultResponse> filterSearchResults(List<DocumentSearchResultResponse> results) {
        if (results == null || results.isEmpty()) {
            return List.of();
        }
        Set<Long> deletedDocumentIds = findDeletedDocumentIds();
        Set<Long> retrievableChunkIds = findRetrievableChunkIds();
        return filterSearchResults(results, deletedDocumentIds, retrievableChunkIds);
    }

    public List<DocumentSearchResultResponse> filterSearchResults(
            List<DocumentSearchResultResponse> results,
            Set<Long> deletedDocumentIds
    ) {
        Set<Long> retrievableChunkIds = findRetrievableChunkIds();
        return filterSearchResults(results, deletedDocumentIds, retrievableChunkIds);
    }

    public List<DocumentSearchResultResponse> filterSearchResults(
            List<DocumentSearchResultResponse> results,
            Set<Long> deletedDocumentIds,
            Set<Long> retrievableChunkIds
    ) {
        if (results == null || results.isEmpty()) {
            return List.of();
        }
        return results.stream()
                .filter(result -> !isDeleted(result.getDocumentId(), deletedDocumentIds))
                .filter(result -> isRetrievableChunk(result.getChunkId(), retrievableChunkIds))
                .toList();
    }

    public List<LexicalCandidate> filterLexicalCandidates(List<LexicalCandidate> candidates) {
        if (candidates == null || candidates.isEmpty()) {
            return List.of();
        }
        Set<Long> deletedDocumentIds = findDeletedDocumentIds();
        Set<Long> retrievableChunkIds = findRetrievableChunkIds();
        return filterLexicalCandidates(candidates, deletedDocumentIds, retrievableChunkIds);
    }

    public List<LexicalCandidate> filterLexicalCandidates(
            List<LexicalCandidate> candidates,
            Set<Long> deletedDocumentIds
    ) {
        Set<Long> retrievableChunkIds = findRetrievableChunkIds();
        return filterLexicalCandidates(candidates, deletedDocumentIds, retrievableChunkIds);
    }

    public List<LexicalCandidate> filterLexicalCandidates(
            List<LexicalCandidate> candidates,
            Set<Long> deletedDocumentIds,
            Set<Long> retrievableChunkIds
    ) {
        if (candidates == null || candidates.isEmpty()) {
            return List.of();
        }
        return candidates.stream()
                .filter(candidate -> !isDeleted(candidate.documentId(), deletedDocumentIds))
                .filter(candidate -> isRetrievableChunk(candidate.chunkId(), retrievableChunkIds))
                .toList();
    }

    public Set<Long> collectDocumentIds(List<DocumentSearchResultResponse> results) {
        return results.stream()
                .map(DocumentSearchResultResponse::getDocumentId)
                .collect(java.util.stream.Collectors.toSet());
    }
}
