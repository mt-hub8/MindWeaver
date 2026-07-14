package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.document.lifecycle.DocumentLifecycleDisplayTexts;
import com.tuoman.ai_task_orchestrator.dto.CollectionAssignmentResponse;
import com.tuoman.ai_task_orchestrator.dto.CollectionDetailResponse;
import com.tuoman.ai_task_orchestrator.dto.CollectionDocumentItemResponse;
import com.tuoman.ai_task_orchestrator.dto.CollectionMembershipResponse;
import com.tuoman.ai_task_orchestrator.dto.CollectionSummaryResponse;
import com.tuoman.ai_task_orchestrator.dto.CreateCollectionRequest;
import com.tuoman.ai_task_orchestrator.entity.DocumentCollectionEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.entity.KnowledgeCollectionEntity;
import com.tuoman.ai_task_orchestrator.enums.CollectionStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentCollectionRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.repository.KnowledgeCollectionRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.ArrayList;
import java.util.List;

@Service
@RequiredArgsConstructor
/**
 * V5.0 知识库分组管理服务。
 *
 * 负责 collection 创建、文档加入/移出和详情查询。它的输出会被 DocumentService、
 * CollectionScopeService、RAG Answer 和 Agent Task 用来展示或解析检索范围。
 *
 * 关键不变量：加入分组不代表文档可检索；TRASHED/PURGED 或未 READY 文档仍必须被
 * scope/lifecycle filter 排除。
 */
public class CollectionService {

    private final KnowledgeCollectionRepository knowledgeCollectionRepository;

    private final DocumentCollectionRepository documentCollectionRepository;

    private final DocumentRepository documentRepository;

    @Transactional
    public CollectionSummaryResponse createCollection(CreateCollectionRequest request) {
        if (request == null || request.getName() == null || request.getName().isBlank()) {
            throw BusinessException.collectionNameRequired();
        }
        String name = request.getName().trim();
        if (knowledgeCollectionRepository.existsByName(name)) {
            throw BusinessException.collectionNameDuplicated();
        }

        KnowledgeCollectionEntity entity = new KnowledgeCollectionEntity();
        entity.setName(name);
        entity.setDescription(trimToNull(request.getDescription()));
        entity.setStatus(CollectionStatus.ACTIVE);
        KnowledgeCollectionEntity saved = knowledgeCollectionRepository.save(entity);
        return toSummaryResponse(saved);
    }

    @Transactional(readOnly = true)
    public List<CollectionSummaryResponse> listCollections() {
        return knowledgeCollectionRepository.findAllByOrderByCreatedAtDesc()
                .stream()
                .map(this::toSummaryResponse)
                .toList();
    }

    @Transactional(readOnly = true)
    public CollectionDetailResponse getCollection(Long collectionId) {
        KnowledgeCollectionEntity collection = findCollectionOrThrow(collectionId);
        List<CollectionDocumentItemResponse> documents = new ArrayList<>();
        for (Long documentId : documentCollectionRepository.findDocumentIdsByCollectionId(collectionId)) {
            documentRepository.findById(documentId).ifPresent(document -> documents.add(toCollectionDocumentItem(document)));
        }
        return new CollectionDetailResponse(
                collection.getId(),
                collection.getName(),
                collection.getDescription(),
                documentCollectionRepository.countByCollectionId(collectionId),
                documentCollectionRepository.countActiveDocumentsByCollectionId(collectionId),
                collection.getCreatedAt(),
                collection.getUpdatedAt(),
                documents
        );
    }

    @Transactional
    public CollectionAssignmentResponse assignDocument(Long collectionId, Long documentId) {
        // membership 只表达“属于该分组”，不改变文档生命周期或索引状态。
        // 垃圾箱中文档可以保留关系，但不会进入问答上下文。
        findCollectionOrThrow(collectionId);
        DocumentEntity document = documentRepository.findById(documentId)
                .orElseThrow(BusinessException::documentNotFound);

        if (documentCollectionRepository.existsByCollectionIdAndDocumentId(collectionId, documentId)) {
            return new CollectionAssignmentResponse(
                    collectionId,
                    documentId,
                    "该文档已加入此分组"
            );
        }

        DocumentCollectionEntity membership = new DocumentCollectionEntity();
        membership.setCollectionId(collectionId);
        membership.setDocumentId(document.getId());
        documentCollectionRepository.save(membership);

        String message = document.getLifecycleStatus() != DocumentLifecycleStatus.ACTIVE
                ? "文档已加入分组。垃圾箱中的文档不会参与问答。"
                : "文档已加入分组";
        return new CollectionAssignmentResponse(collectionId, documentId, message);
    }

    @Transactional
    public CollectionAssignmentResponse removeDocument(Long collectionId, Long documentId) {
        findCollectionOrThrow(collectionId);
        if (!documentRepository.existsById(documentId)) {
            throw BusinessException.documentNotFound();
        }

        if (!documentCollectionRepository.existsByCollectionIdAndDocumentId(collectionId, documentId)) {
            return new CollectionAssignmentResponse(
                    collectionId,
                    documentId,
                    "该文档未加入此分组"
            );
        }

        documentCollectionRepository.deleteByCollectionIdAndDocumentId(collectionId, documentId);
        return new CollectionAssignmentResponse(collectionId, documentId, "该文档已从分组移出");
    }

    @Transactional(readOnly = true)
    public KnowledgeCollectionEntity findCollectionOrThrow(Long collectionId) {
        return knowledgeCollectionRepository.findById(collectionId)
                .orElseThrow(BusinessException::collectionNotFound);
    }

    @Transactional(readOnly = true)
    public List<CollectionMembershipResponse> findMembershipsByDocumentId(Long documentId) {
        return documentCollectionRepository.findCollectionSummariesByDocumentId(documentId)
                .stream()
                .map(row -> new CollectionMembershipResponse((Long) row[0], (String) row[1]))
                .toList();
    }

    private CollectionSummaryResponse toSummaryResponse(KnowledgeCollectionEntity collection) {
        return new CollectionSummaryResponse(
                collection.getId(),
                collection.getName(),
                collection.getDescription(),
                documentCollectionRepository.countByCollectionId(collection.getId()),
                documentCollectionRepository.countActiveDocumentsByCollectionId(collection.getId()),
                collection.getCreatedAt(),
                collection.getUpdatedAt()
        );
    }

    private CollectionDocumentItemResponse toCollectionDocumentItem(DocumentEntity document) {
        DocumentLifecycleStatus lifecycleStatus = document.getLifecycleStatus() == null
                ? DocumentLifecycleStatus.ACTIVE
                : document.getLifecycleStatus();
        boolean canAsk = lifecycleStatus == DocumentLifecycleStatus.ACTIVE
                && document.getStatus() == DocumentStatus.READY;
        return new CollectionDocumentItemResponse(
                document.getId(),
                document.getOriginalFilename(),
                lifecycleStatus.name(),
                DocumentLifecycleDisplayTexts.displayStatus(lifecycleStatus),
                canAsk,
                true,
                DocumentLifecycleDisplayTexts.lifecycleHint(
                        lifecycleStatus,
                        document.getStatus(),
                        canAsk
                )
        );
    }

    private String trimToNull(String value) {
        if (value == null || value.isBlank()) {
            return null;
        }
        return value.trim();
    }
}
