package com.tuoman.ai_task_orchestrator.grounding;

import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.queryunderstanding.QueryUnderstandingResult;
import com.tuoman.ai_task_orchestrator.queryunderstanding.RetrievalRoutingDecision;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.rerank.RagTwoStageRetrievalService.RagRetrievedChunk;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;

import java.util.ArrayList;
import java.util.Comparator;
import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.UUID;
import java.util.function.Function;
import java.util.stream.Collectors;

@Service
@RequiredArgsConstructor
public class GroundedContextAssembler {

    private static final int DEFAULT_CONTEXT_BUDGET = 6000;

    private final DocumentChunkRepository chunkRepository;

    private final DocumentRepository documentRepository;

    public GroundedContextBundle assemble(
            String query,
            List<RagRetrievedChunk> retrievedChunks,
            QueryUnderstandingResult understanding,
            RetrievalRoutingDecision routingDecision,
            Integer contextBudget,
            AnswerContractMode mode
    ) {
        int budget = contextBudget == null || contextBudget < 500 ? DEFAULT_CONTEXT_BUDGET : contextBudget;
        List<RagRetrievedChunk> source = retrievedChunks == null ? List.of() : retrievedChunks;
        Map<Long, DocumentChunkEntity> chunkMap = loadChunks(source);
        Map<Long, DocumentEntity> documentMap = loadDocuments(source, chunkMap);
        Map<String, RagRetrievedChunk> deduped = new LinkedHashMap<>();
        for (RagRetrievedChunk chunk : source) {
            String key = dedupeKey(chunk, chunkMap.get(chunk.chunkId()));
            deduped.putIfAbsent(key, chunk);
        }

        List<RagRetrievedChunk> ordered = deduped.values().stream()
                .sorted(Comparator.comparingInt(RagRetrievedChunk::rerankedRank))
                .toList();
        List<GroundedContextChunk> finalChunks = new ArrayList<>();
        List<Citation> citations = new ArrayList<>();
        List<String> warnings = new ArrayList<>();
        int used = 0;
        int citationIndex = 1;
        boolean truncated = false;
        Map<Long, Integer> perDocumentCount = new HashMap<>();

        for (RagRetrievedChunk retrieved : ordered) {
            if (retrieved == null || retrieved.chunkId() == null) {
                continue;
            }
            int docCount = perDocumentCount.getOrDefault(retrieved.documentId(), 0);
            if (docCount >= 4) {
                warnings.add("document " + retrieved.documentId() + " capped to avoid context domination");
                continue;
            }
            DocumentChunkEntity chunk = chunkMap.get(retrieved.chunkId());
            DocumentEntity document = documentMap.get(retrieved.documentId());
            String text = retrieved.content() == null ? "" : retrieved.content();
            int remaining = budget - used;
            if (remaining <= 0) {
                truncated = true;
                break;
            }
            boolean chunkTruncated = text.length() > remaining;
            String finalText = chunkTruncated ? text.substring(0, Math.max(0, remaining)) : text;
            if (finalText.isBlank()) {
                continue;
            }
            String citationKey = "[" + citationIndex + "]";
            GroundedContextChunk contextChunk = GroundedContextChunk.builder()
                    .chunkId(retrieved.chunkId())
                    .documentId(retrieved.documentId())
                    .documentTitle(documentTitle(retrieved, document))
                    .collectionId(chunk == null ? null : chunk.getCollectionId())
                    .version(resolveVersion(chunk, document))
                    .docType(resolveDocType(chunk, document))
                    .sectionPath(resolveSectionPath(chunk, retrieved))
                    .chunkType(chunk == null || chunk.getChunkType() == null ? null : chunk.getChunkType().name())
                    .text(finalText)
                    .rank(retrieved.rerankedRank())
                    .score(resolveScore(retrieved))
                    .retrievalSource(resolveRetrievalSource(retrieved))
                    .directHit(true)
                    .expanded(false)
                    .parent(false)
                    .adjacent(false)
                    .citationKey(citationKey)
                    .truncated(chunkTruncated)
                    .metadata(Map.of(
                            "answerContractMode", mode == null ? AnswerContractMode.defaultMode().name() : mode.name(),
                            "queryType", understanding == null || understanding.getQueryType() == null ? "" : understanding.getQueryType().name(),
                            "routingStrategy", routingDecision == null || routingDecision.getStrategy() == null ? "" : routingDecision.getStrategy().name()
                    ))
                    .build();
            finalChunks.add(contextChunk);
            citations.add(toCitation(contextChunk));
            used += finalText.length();
            perDocumentCount.put(retrieved.documentId(), docCount + 1);
            citationIndex++;
            if (chunkTruncated) {
                truncated = true;
                break;
            }
        }

        return GroundedContextBundle.builder()
                .contextId(UUID.randomUUID().toString())
                .query(query)
                .chunks(finalChunks)
                .citations(citations)
                .contextBudget(budget)
                .usedChars(used)
                .usedTokensEstimate(Math.max(1, used / 4))
                .truncated(truncated)
                .warnings(warnings)
                .build();
    }

    private Map<Long, DocumentChunkEntity> loadChunks(List<RagRetrievedChunk> chunks) {
        List<Long> ids = chunks.stream().map(RagRetrievedChunk::chunkId).filter(Objects::nonNull).distinct().toList();
        if (ids.isEmpty()) {
            return Map.of();
        }
        return chunkRepository.findAllById(ids).stream()
                .collect(Collectors.toMap(DocumentChunkEntity::getId, Function.identity()));
    }

    private Map<Long, DocumentEntity> loadDocuments(List<RagRetrievedChunk> chunks, Map<Long, DocumentChunkEntity> chunkMap) {
        List<Long> ids = chunks.stream()
                .map(chunk -> chunk.documentId() != null ? chunk.documentId() : chunkMap.get(chunk.chunkId()))
                .map(value -> value instanceof DocumentChunkEntity entity ? entity.getDocumentId() : value)
                .filter(Long.class::isInstance)
                .map(Long.class::cast)
                .distinct()
                .toList();
        if (ids.isEmpty()) {
            return Map.of();
        }
        return documentRepository.findAllById(ids).stream()
                .collect(Collectors.toMap(DocumentEntity::getId, Function.identity()));
    }

    private String dedupeKey(RagRetrievedChunk retrieved, DocumentChunkEntity chunk) {
        if (chunk != null && chunk.getNormalizedContentHash() != null) {
            return "hash:" + chunk.getNormalizedContentHash();
        }
        return "chunk:" + retrieved.chunkId();
    }

    private Citation toCitation(GroundedContextChunk chunk) {
        return Citation.builder()
                .citationId(chunk.getCitationKey())
                .citationKey(chunk.getCitationKey())
                .chunkId(chunk.getChunkId())
                .documentId(chunk.getDocumentId())
                .documentTitle(chunk.getDocumentTitle())
                .collectionId(chunk.getCollectionId())
                .version(chunk.getVersion())
                .sectionPath(chunk.getSectionPath())
                .quoteSnippet(snippet(chunk.getText(), 300))
                .supportLevel(SupportLevel.UNKNOWN)
                .verificationStatus(VerificationStatus.PENDING)
                .build();
    }

    private String documentTitle(RagRetrievedChunk retrieved, DocumentEntity document) {
        if (document != null && document.getOriginalFilename() != null) {
            return document.getOriginalFilename();
        }
        return retrieved.documentTitle();
    }

    private String resolveVersion(DocumentChunkEntity chunk, DocumentEntity document) {
        if (chunk != null && chunk.getVersion() != null) {
            return chunk.getVersion();
        }
        return document == null ? null : document.getVersion();
    }

    private String resolveDocType(DocumentChunkEntity chunk, DocumentEntity document) {
        if (chunk != null && chunk.getDocType() != null) {
            return chunk.getDocType().name();
        }
        return document == null || document.getDocType() == null ? null : document.getDocType().name();
    }

    private String resolveSectionPath(DocumentChunkEntity chunk, RagRetrievedChunk retrieved) {
        if (chunk != null && chunk.getSectionPath() != null) {
            return chunk.getSectionPath();
        }
        if (chunk != null && chunk.getHeadingPath() != null) {
            return chunk.getHeadingPath();
        }
        return retrieved.documentTitle();
    }

    private Double resolveScore(RagRetrievedChunk chunk) {
        if (chunk.rerankScore() != null) {
            return chunk.rerankScore();
        }
        if (chunk.fusionScore() != null) {
            return chunk.fusionScore();
        }
        return chunk.originalScore();
    }

    private String resolveRetrievalSource(RagRetrievedChunk chunk) {
        if (Boolean.TRUE.equals(chunk.denseHit()) && Boolean.TRUE.equals(chunk.lexicalHit())) {
            return "HYBRID";
        }
        if (Boolean.TRUE.equals(chunk.lexicalHit())) {
            return "KEYWORD";
        }
        return "VECTOR";
    }

    private String snippet(String text, int max) {
        if (text == null) {
            return "";
        }
        return text.length() <= max ? text : text.substring(0, max);
    }
}
