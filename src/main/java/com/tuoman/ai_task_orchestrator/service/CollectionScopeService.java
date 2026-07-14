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
/**
 * V5.0 scoped retrieval 范围解析服务。
 *
 * 把用户选择的 collectionId 转换为 RAG/Agent 可用的 RetrievalScope。
 * 它同时检查 membership、生命周期和是否存在可检索 chunk，避免空分组或全删除分组退化为全库搜索。
 */
public class CollectionScopeService {

    private final CollectionService collectionService;

    private final DocumentCollectionRepository documentCollectionRepository;

    private final DocumentRepository documentRepository;

    @Transactional(readOnly = true)
    public CollectionAskScope resolveForAsk(Long collectionId) {
        if (collectionId == null) {
            return CollectionAskScope.notApplicable();
        }

        // 用户显式选择 collection 时，scope 优先级高于任何自动推断。
        // 如果该分组不可问，返回 noContext 原因，而不是偷偷扩大到全部文档。
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
        // RetrievalScope 是 AppRetrievalService / Agent Tool 的统一输入。
        // allowedDocumentIds 为空表示该 collection 无可检索内容，而不是“不过滤”。
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
