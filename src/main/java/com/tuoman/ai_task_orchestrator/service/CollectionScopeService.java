package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.entity.KnowledgeCollectionEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentCollectionRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.retrieval.CollectionAskEmptyReason;
import com.tuoman.ai_task_orchestrator.retrieval.CollectionAskScope;
import com.tuoman.ai_task_orchestrator.retrieval.RetrievalScope;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.HashSet;
import java.util.List;
import java.util.Set;

@Service
@RequiredArgsConstructor
public class CollectionScopeService {

    private final CollectionService collectionService;

    private final DocumentCollectionRepository documentCollectionRepository;

    private final DocumentRepository documentRepository;

    @Transactional(readOnly = true)
    public CollectionAskScope resolveForAsk(Long collectionId) {
        if (collectionId == null) {
            return CollectionAskScope.notApplicable();
        }

        KnowledgeCollectionEntity collection = collectionService.findCollectionOrThrow(collectionId);
        List<Long> memberDocumentIds = documentCollectionRepository.findDocumentIdsByCollectionId(collectionId);
        Set<Long> memberIds = new HashSet<>(memberDocumentIds);

        if (memberIds.isEmpty()) {
            return new CollectionAskScope(
                    collectionId,
                    collection.getName(),
                    CollectionAskEmptyReason.NO_DOCUMENTS,
                    Set.of(),
                    Set.of(),
                    "当前分组下没有可用于问答的文档，请先添加已启用文档。"
            );
        }

        boolean anyActive = memberDocumentIds.stream()
                .map(documentRepository::findById)
                .filter(java.util.Optional::isPresent)
                .map(java.util.Optional::get)
                .anyMatch(document -> document.getLifecycleStatus() == DocumentLifecycleStatus.ACTIVE);

        if (!anyActive) {
            return new CollectionAskScope(
                    collectionId,
                    collection.getName(),
                    CollectionAskEmptyReason.ALL_DELETED,
                    memberIds,
                    Set.of(),
                    "当前分组中的文档均已删除，无法用于问答。"
            );
        }

        Set<Long> askableDocumentIds = new HashSet<>(
                documentCollectionRepository.findAskableDocumentIdsByCollectionId(collectionId)
        );

        if (askableDocumentIds.isEmpty()) {
            return new CollectionAskScope(
                    collectionId,
                    collection.getName(),
                    CollectionAskEmptyReason.NO_RETRIEVABLE_CHUNKS,
                    memberIds,
                    Set.of(),
                    "当前分组暂无可用的最新索引片段，请重新建立索引或上传文档。"
            );
        }

        return new CollectionAskScope(
                collectionId,
                collection.getName(),
                CollectionAskEmptyReason.NONE,
                memberIds,
                askableDocumentIds,
                null
        );
    }

    @Transactional(readOnly = true)
    public RetrievalScope resolveRetrievalScope(Long collectionId) {
        CollectionAskScope askScope = resolveForAsk(collectionId);
        if (collectionId == null) {
            return RetrievalScope.allDocuments();
        }
        if (askScope.shouldSkipRetrieval()) {
            return RetrievalScope.collection(
                    askScope.collectionId(),
                    askScope.collectionName(),
                    Set.of()
            );
        }
        return RetrievalScope.collection(
                askScope.collectionId(),
                askScope.collectionName(),
                askScope.askableDocumentIds()
        );
    }

    @Transactional(readOnly = true)
    public void validateCollectionExists(Long collectionId) {
        if (collectionId == null) {
            return;
        }
        collectionService.findCollectionOrThrow(collectionId);
    }
}
