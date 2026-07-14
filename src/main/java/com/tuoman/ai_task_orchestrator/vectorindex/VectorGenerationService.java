package com.tuoman.ai_task_orchestrator.vectorindex;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.config.ChunkingProperties;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.entity.VectorIndexGenerationEntity;
import com.tuoman.ai_task_orchestrator.enums.VectorGenerationStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.repository.VectorIndexGenerationRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.LocalDateTime;
import java.util.List;
import java.util.Optional;

/**
 * V16 向量索引 generation 状态机服务。
 *
 * BUILDING 表示新索引正在构建，ACTIVE 表示当前可检索，RETIRED 表示被新索引替换，
 * FAILED 表示构建失败且不能参与检索。
 *
 * 关键不变量：reindex 不能直接覆盖旧 vector；新 generation 只有完整构建并激活后，
 * 才能替换旧 ACTIVE generation。
 */
@Service
@RequiredArgsConstructor
public class VectorGenerationService {

    private final VectorIndexGenerationRepository generationRepository;

    private final DocumentRepository documentRepository;

    private final ChunkingProperties chunkingProperties;

    @Transactional
    public VectorIndexGenerationEntity ensureActiveGeneration(
            Long collectionId,
            Long documentId,
            String embeddingModel,
            Integer embeddingDimension
    ) {
        // 初始导入没有显式 reindex 流程时，确保存在 generation=1 的 ACTIVE 记录。
        // 后续检索会通过 active generation filter 避免新旧向量混召回。
        Optional<VectorIndexGenerationEntity> existing = findActive(documentId, collectionId);
        if (existing.isPresent()) {
            return existing.get();
        }
        VectorIndexGenerationEntity entity = new VectorIndexGenerationEntity();
        entity.setCollectionId(collectionId);
        entity.setDocumentId(documentId);
        entity.setGeneration(1L);
        entity.setStatus(VectorGenerationStatus.ACTIVE);
        entity.setEmbeddingModel(embeddingModel);
        entity.setEmbeddingDimension(embeddingDimension);
        entity.setChunkingStrategy(chunkingProperties.getStrategy() == null
                ? null
                : chunkingProperties.getStrategy().name());
        entity.setActivatedAt(LocalDateTime.now());
        entity.setCompletedAt(LocalDateTime.now());
        entity.setSummaryMessage("initial active generation");
        return generationRepository.save(entity);
    }

    @Transactional
    public VectorIndexGenerationEntity beginBuildingGeneration(
            Long collectionId,
            Long documentId,
            Long generation,
            String embeddingModel,
            Integer embeddingDimension
    ) {
        // BUILDING generation 允许写入和校验，但不应参与普通 retrieval。
        // 这让 reindex 可以在后台完成，不影响旧 ACTIVE generation 对外服务。
        VectorIndexGenerationEntity entity = new VectorIndexGenerationEntity();
        entity.setCollectionId(collectionId);
        entity.setDocumentId(documentId);
        entity.setGeneration(generation);
        entity.setStatus(VectorGenerationStatus.BUILDING);
        entity.setEmbeddingModel(embeddingModel);
        entity.setEmbeddingDimension(embeddingDimension);
        entity.setChunkingStrategy(chunkingProperties.getStrategy() == null
                ? null
                : chunkingProperties.getStrategy().name());
        entity.setSummaryMessage("building generation " + generation);
        return generationRepository.save(entity);
    }

    @Transactional
    public VectorIndexGenerationEntity activateGeneration(Long collectionId, Long documentId, Long generation) {
        // 激活新 generation 时先把旧 ACTIVE 标为 RETIRED，再激活 BUILDING。
        // 如果构建阶段失败，此方法不会执行，旧 ACTIVE generation 继续提供检索。
        VectorIndexGenerationEntity building = generationRepository
                .findByDocumentIdAndGeneration(documentId, generation)
                .orElseThrow(() -> BusinessException.vectorGenerationInvalid("BUILDING generation 不存在"));

        List<VectorIndexGenerationEntity> activeList = generationRepository.findByDocumentIdAndStatus(
                documentId,
                VectorGenerationStatus.ACTIVE
        );
        for (VectorIndexGenerationEntity active : activeList) {
            active.setStatus(VectorGenerationStatus.RETIRED);
            active.setRetiredAt(LocalDateTime.now());
            generationRepository.save(active);
        }

        building.setStatus(VectorGenerationStatus.ACTIVE);
        building.setActivatedAt(LocalDateTime.now());
        building.setCompletedAt(LocalDateTime.now());
        building.setSummaryMessage("activated generation " + generation);
        return generationRepository.save(building);
    }

    @Transactional
    public void markGenerationFailed(Long documentId, Long generation, String message) {
        // FAILED 只标记新 generation 的构建失败，不能级联修改旧 ACTIVE generation。
        // 这是 reindex 可回滚性的核心约束。
        generationRepository.findByDocumentIdAndGeneration(documentId, generation).ifPresent(entity -> {
            entity.setStatus(VectorGenerationStatus.FAILED);
            entity.setSummaryMessage(message);
            entity.setCompletedAt(LocalDateTime.now());
            generationRepository.save(entity);
        });
    }

    public VectorIndexGenerationEntity resolveWritableGeneration(
            Long collectionId,
            Long documentId,
            Long requestedGeneration
    ) {
        if (requestedGeneration != null) {
            return generationRepository.findByDocumentIdAndGeneration(documentId, requestedGeneration)
                    .orElseGet(() -> beginBuildingGeneration(
                            collectionId,
                            documentId,
                            requestedGeneration,
                            null,
                            null
                    ));
        }
        return ensureActiveGeneration(collectionId, documentId, null, null);
    }

    public Optional<Long> getActiveGeneration(Long documentId) {
        return findActive(documentId, null).map(VectorIndexGenerationEntity::getGeneration);
    }

    public Optional<VectorIndexGenerationEntity> findActive(Long documentId, Long collectionId) {
        if (documentId != null) {
            return generationRepository.findFirstByDocumentIdAndStatusOrderByGenerationDesc(
                    documentId,
                    VectorGenerationStatus.ACTIVE
            );
        }
        if (collectionId != null) {
            return generationRepository.findFirstByCollectionIdAndDocumentIdIsNullAndStatusOrderByGenerationDesc(
                    collectionId,
                    VectorGenerationStatus.ACTIVE
            );
        }
        return Optional.empty();
    }

    public List<VectorIndexGenerationEntity> findRetiredGenerations(Long documentId) {
        return generationRepository.findByDocumentIdAndStatus(documentId, VectorGenerationStatus.RETIRED);
    }

    public DocumentEntity requireDocument(Long documentId) {
        return documentRepository.findById(documentId)
                .orElseThrow(BusinessException::documentNotFound);
    }
}
