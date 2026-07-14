package com.tuoman.ai_task_orchestrator.retrieval;

import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.Comparator;
import java.util.List;
import java.util.Locale;
import java.util.Set;

/**
 * 轻量 keyword retriever 实现。
 *
 * 这是 V15 Hybrid Retrieval 的本地字面召回补充，不等价于完整搜索引擎或标准 BM25。
 * 它按标题、sectionPath、正文和技术符号命中进行启发式打分，输出给 RRF 与 dense 结果融合。
 *
 * 关键约束：每个候选 chunk 都必须通过 RetrievalFilterService.matchesChunk，
 * 不能为了命中关键词绕过 collection、version、status 或 TRASHED/PURGED 过滤。
 */
@Component
@RequiredArgsConstructor
public class SimpleKeywordRetriever implements KeywordRetriever {

    public static final String PROVIDER = "simple-keyword";

    private final RetrievalFilterService retrievalFilterService;

    private final DocumentChunkRepository documentChunkRepository;

    private final DocumentRepository documentRepository;

    @Override
    public KeywordRetrievalResponse search(String query, RetrievalFilter filter, int topK) {
        // keyword retrieval 以 allowedDocs 为硬范围，再逐 chunk 应用 matchesChunk。
        // 这样两路召回的过滤语义保持一致，避免只在 dense 一路做 metadata pre-filter。
        long startedAt = System.nanoTime();
        if (query == null || query.isBlank()) {
            return new KeywordRetrievalResponse(List.of(), 0L);
        }
        Set<Long> allowedDocs = retrievalFilterService.resolveAllowedDocumentIds(filter == null ? RetrievalFilter.empty() : filter);
        List<String> tokens = tokenize(query);
        List<KeywordCandidate> scored = new ArrayList<>();

        for (Long documentId : allowedDocs) {
            DocumentEntity document = documentRepository.findById(documentId).orElse(null);
            List<DocumentChunkEntity> chunks = documentChunkRepository.findByDocumentIdOrderByChunkIndexAsc(documentId);
            for (DocumentChunkEntity chunk : chunks) {
                if (!retrievalFilterService.matchesChunk(chunk, document, filter == null ? RetrievalFilter.empty() : filter)) {
                    continue;
                }
                double score = scoreChunk(query, tokens, chunk);
                if (score > 0) {
                    scored.add(new KeywordCandidate(
                            0,
                            documentId,
                            document != null ? document.getOriginalFilename() : null,
                            chunk.getId(),
                            chunk.getSectionPath() != null ? chunk.getSectionPath() : chunk.getHeadingPath(),
                            chunk.getContent(),
                            score,
                            describeMatch(query, chunk)
                    ));
                }
            }
        }

        scored.sort(Comparator.comparingDouble(KeywordCandidate::score).reversed());
        List<KeywordCandidate> limited = new ArrayList<>();
        for (int i = 0; i < Math.min(topK, scored.size()); i++) {
            KeywordCandidate candidate = scored.get(i);
            limited.add(new KeywordCandidate(
                    i + 1,
                    candidate.documentId(),
                    candidate.documentTitle(),
                    candidate.chunkId(),
                    candidate.sectionPath(),
                    candidate.content(),
                    candidate.score(),
                    candidate.matchReason()
            ));
        }
        long latencyMs = (System.nanoTime() - startedAt) / 1_000_000;
        return new KeywordRetrievalResponse(limited, latencyMs);
    }

    @Override
    public String name() {
        return PROVIDER;
    }

    private double scoreChunk(String query, List<String> tokens, DocumentChunkEntity chunk) {
        // 这里是可解释的启发式 keyword score：完整 query 命中、标题命中、路径命中和正文 token 命中
        // 分别加权。该分数只用于 keyword 内部排序，后续不会直接与 vector similarity 比较。
        String content = chunk.getContent() == null ? "" : chunk.getContent().toLowerCase(Locale.ROOT);
        String section = chunk.getSectionPath() == null ? "" : chunk.getSectionPath().toLowerCase(Locale.ROOT);
        String title = chunk.getSectionTitle() == null ? "" : chunk.getSectionTitle().toLowerCase(Locale.ROOT);
        double score = 0.0;
        String lowerQuery = query.toLowerCase(Locale.ROOT);
        if (content.contains(lowerQuery)) {
            score += 3.0;
        }
        for (String token : tokens) {
            if (title.contains(token)) {
                score += 2.0;
            }
            if (section.contains(token)) {
                score += 1.5;
            }
            if (content.contains(token)) {
                score += 1.0;
            }
        }
        if (looksLikeSymbol(query) && content.contains(lowerQuery)) {
            score += 2.0;
        }
        return score;
    }

    private String describeMatch(String query, DocumentChunkEntity chunk) {
        String lowerQuery = query.toLowerCase(Locale.ROOT);
        if (chunk.getSectionTitle() != null && chunk.getSectionTitle().toLowerCase(Locale.ROOT).contains(lowerQuery)) {
            return "title_match";
        }
        if (chunk.getSectionPath() != null && chunk.getSectionPath().toLowerCase(Locale.ROOT).contains(lowerQuery)) {
            return "section_path_match";
        }
        return "text_match";
    }

    private boolean looksLikeSymbol(String query) {
        return query.contains(".") || query.contains("_") || Character.isUpperCase(query.charAt(0));
    }

    private List<String> tokenize(String query) {
        List<String> tokens = new ArrayList<>();
        for (String part : query.toLowerCase(Locale.ROOT).split("[\\s，。、；：？！,.;:!?/_-]+")) {
            if (part.length() >= 2) {
                tokens.add(part);
            }
        }
        if (tokens.isEmpty() && query.length() >= 2) {
            tokens.add(query.toLowerCase(Locale.ROOT));
        }
        return tokens;
    }
}
