package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.DocumentSearchResultResponse;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.hybrid.LexicalCandidate;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.HashSet;
import java.util.List;
import java.util.Set;
import java.util.stream.Collectors;

@Service
@RequiredArgsConstructor
public class DocumentLifecycleFilterService {

    private final DocumentRepository documentRepository;

    @Transactional(readOnly = true)
    public Set<Long> findDeletedDocumentIds() {
        return new HashSet<>(documentRepository.findIdsByLifecycleStatus(DocumentLifecycleStatus.DELETED));
    }

    public boolean isDeleted(Long documentId, Set<Long> deletedDocumentIds) {
        return documentId != null && deletedDocumentIds.contains(documentId);
    }

    public List<DocumentSearchResultResponse> filterSearchResults(
            List<DocumentSearchResultResponse> results,
            Set<Long> deletedDocumentIds
    ) {
        if (results == null || results.isEmpty()) {
            return List.of();
        }
        return results.stream()
                .filter(result -> !isDeleted(result.getDocumentId(), deletedDocumentIds))
                .toList();
    }

    public List<LexicalCandidate> filterLexicalCandidates(
            List<LexicalCandidate> candidates,
            Set<Long> deletedDocumentIds
    ) {
        if (candidates == null || candidates.isEmpty()) {
            return List.of();
        }
        return candidates.stream()
                .filter(candidate -> !isDeleted(candidate.documentId(), deletedDocumentIds))
                .toList();
    }

    public Set<Long> collectDocumentIds(List<DocumentSearchResultResponse> results) {
        return results.stream()
                .map(DocumentSearchResultResponse::getDocumentId)
                .collect(Collectors.toSet());
    }
}
