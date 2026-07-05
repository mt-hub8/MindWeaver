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
    public Set<Long> findNonRetrievableDocumentIds() {
        return new HashSet<>(documentRepository.findNonRetrievableDocumentIds());
    }

    @Transactional(readOnly = true)
    public Set<Long> findDeletedDocumentIds() {
        return findNonRetrievableDocumentIds();
    }

    public boolean isNonRetrievable(Long documentId, Set<Long> nonRetrievableDocumentIds) {
        return documentId != null && nonRetrievableDocumentIds.contains(documentId);
    }

    public boolean isDeleted(Long documentId, Set<Long> deletedDocumentIds) {
        return isNonRetrievable(documentId, deletedDocumentIds);
    }

    @Transactional(readOnly = true)
    public Set<Long> findRetrievableChunkIds() {
        return new HashSet<>(documentChunkRepository.findRetrievableChunkIds());
    }

    public boolean isRetrievableChunk(Long chunkId, Set<Long> retrievableChunkIds) {
        return chunkId != null && retrievableChunkIds.contains(chunkId);
    }

    public List<DocumentSearchResultResponse> filterSearchResults(List<DocumentSearchResultResponse> results) {
        return filterSearchResults(results, null);
    }

    public List<DocumentSearchResultResponse> filterSearchResults(
            List<DocumentSearchResultResponse> results,
            Set<Long> allowedDocumentIds
    ) {
        if (results == null || results.isEmpty()) {
            return List.of();
        }
        Set<Long> deletedDocumentIds = findDeletedDocumentIds();
        Set<Long> retrievableChunkIds = findRetrievableChunkIds();
        return filterSearchResults(results, deletedDocumentIds, retrievableChunkIds, allowedDocumentIds);
    }

    public List<DocumentSearchResultResponse> filterSearchResults(
            List<DocumentSearchResultResponse> results,
            Set<Long> deletedDocumentIds,
            Set<Long> retrievableChunkIds
    ) {
        return filterSearchResults(results, deletedDocumentIds, retrievableChunkIds, null);
    }

    public List<DocumentSearchResultResponse> filterSearchResults(
            List<DocumentSearchResultResponse> results,
            Set<Long> deletedDocumentIds,
            Set<Long> retrievableChunkIds,
            Set<Long> allowedDocumentIds
    ) {
        if (results == null || results.isEmpty()) {
            return List.of();
        }
        return results.stream()
                .filter(result -> isAllowedDocument(result.getDocumentId(), allowedDocumentIds))
                .filter(result -> !isDeleted(result.getDocumentId(), deletedDocumentIds))
                .filter(result -> isRetrievableChunk(result.getChunkId(), retrievableChunkIds))
                .toList();
    }

    @Deprecated
    public List<DocumentSearchResultResponse> filterSearchResultsLegacy(
            List<DocumentSearchResultResponse> results,
            Set<Long> deletedDocumentIds
    ) {
        Set<Long> retrievableChunkIds = findRetrievableChunkIds();
        return filterSearchResults(results, deletedDocumentIds, retrievableChunkIds, null);
    }

    public List<LexicalCandidate> filterLexicalCandidates(List<LexicalCandidate> candidates) {
        return filterLexicalCandidates(candidates, null);
    }

    public List<LexicalCandidate> filterLexicalCandidates(
            List<LexicalCandidate> candidates,
            Set<Long> allowedDocumentIds
    ) {
        if (candidates == null || candidates.isEmpty()) {
            return List.of();
        }
        Set<Long> deletedDocumentIds = findDeletedDocumentIds();
        Set<Long> retrievableChunkIds = findRetrievableChunkIds();
        return filterLexicalCandidates(candidates, deletedDocumentIds, retrievableChunkIds, allowedDocumentIds);
    }

    public List<LexicalCandidate> filterLexicalCandidates(
            List<LexicalCandidate> candidates,
            Set<Long> deletedDocumentIds,
            Set<Long> retrievableChunkIds
    ) {
        return filterLexicalCandidates(candidates, deletedDocumentIds, retrievableChunkIds, null);
    }

    public List<LexicalCandidate> filterLexicalCandidates(
            List<LexicalCandidate> candidates,
            Set<Long> deletedDocumentIds,
            Set<Long> retrievableChunkIds,
            Set<Long> allowedDocumentIds
    ) {
        if (candidates == null || candidates.isEmpty()) {
            return List.of();
        }
        return candidates.stream()
                .filter(candidate -> isAllowedDocument(candidate.documentId(), allowedDocumentIds))
                .filter(candidate -> !isDeleted(candidate.documentId(), deletedDocumentIds))
                .filter(candidate -> isRetrievableChunk(candidate.chunkId(), retrievableChunkIds))
                .toList();
    }

    public boolean isAllowedDocument(Long documentId, Set<Long> allowedDocumentIds) {
        if (allowedDocumentIds == null || allowedDocumentIds.isEmpty()) {
            return true;
        }
        return documentId != null && allowedDocumentIds.contains(documentId);
    }

    public Set<Long> collectDocumentIds(List<DocumentSearchResultResponse> results) {
        return results.stream()
                .map(DocumentSearchResultResponse::getDocumentId)
                .collect(java.util.stream.Collectors.toSet());
    }
}
