package com.tuoman.ai_task_orchestrator.rerank;

import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.Comparator;
import java.util.List;
import java.util.Locale;

@Component
@RequiredArgsConstructor
public class SimpleHeuristicReranker implements Reranker {

    public static final String PROVIDER = "heuristic";

    private final DocumentChunkRepository documentChunkRepository;

    @Override
    public RerankResponse rerank(RerankRequest request) {
        long startedAt = System.nanoTime();
        String query = request.query() == null ? "" : request.query().toLowerCase(Locale.ROOT);
        List<ScoredCandidate> scored = new ArrayList<>();
        for (RerankCandidate candidate : request.candidates()) {
            double boost = 0.0;
            DocumentChunkEntity chunk = candidate.chunkId() == null
                    ? null
                    : documentChunkRepository.findById(candidate.chunkId()).orElse(null);
            if (chunk != null) {
                if (chunk.getSectionTitle() != null && chunk.getSectionTitle().toLowerCase(Locale.ROOT).contains(query)) {
                    boost += 0.3;
                }
                if (chunk.getSectionPath() != null && chunk.getSectionPath().toLowerCase(Locale.ROOT).contains(query)) {
                    boost += 0.2;
                }
                if (chunk.getVersion() != null && query.contains(chunk.getVersion().toLowerCase(Locale.ROOT))) {
                    boost += 0.2;
                }
            }
            if (candidate.content() != null && candidate.content().toLowerCase(Locale.ROOT).contains(query)) {
                boost += 0.2;
            }
            double base = candidate.originalScore() == null ? 0.0 : candidate.originalScore();
            scored.add(new ScoredCandidate(candidate, base + boost));
        }
        scored.sort(Comparator.comparingDouble(ScoredCandidate::score).reversed());
        List<RerankedItem> items = new ArrayList<>();
        int limit = Math.min(request.finalTopK(), scored.size());
        for (int i = 0; i < limit; i++) {
            ScoredCandidate scoredCandidate = scored.get(i);
            RerankCandidate candidate = scoredCandidate.candidate();
            items.add(new RerankedItem(
                    i + 1,
                    candidate.originalRank(),
                    candidate.documentId(),
                    candidate.documentTitle(),
                    candidate.chunkId(),
                    candidate.content(),
                    candidate.originalScore(),
                    scoredCandidate.score()
            ));
        }
        long latencyMs = (System.nanoTime() - startedAt) / 1_000_000;
        return new RerankResponse(items, PROVIDER, latencyMs);
    }

    @Override
    public String name() {
        return PROVIDER;
    }

    private record ScoredCandidate(RerankCandidate candidate, double score) {
    }
}
