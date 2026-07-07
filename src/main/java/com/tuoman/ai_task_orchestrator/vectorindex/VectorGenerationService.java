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
