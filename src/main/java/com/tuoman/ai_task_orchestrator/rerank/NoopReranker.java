package com.tuoman.ai_task_orchestrator.rerank;

import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.List;

@Component
public class NoopReranker implements Reranker {

    public static final String PROVIDER = "noop";

    @Override
    public RerankResponse rerank(RerankRequest request) {
        List<RerankedItem> items = new ArrayList<>();
        List<RerankCandidate> candidates = request.candidates() == null ? List.of() : request.candidates();
        int limit = Math.min(request.finalTopK(), candidates.size());
        for (int i = 0; i < limit; i++) {
            RerankCandidate candidate = candidates.get(i);
            items.add(new RerankedItem(
                    i + 1,
                    candidate.originalRank(),
                    candidate.documentId(),
                    candidate.documentTitle(),
                    candidate.chunkId(),
                    candidate.content(),
                    candidate.originalScore(),
                    candidate.originalScore() == null ? 0.0 : candidate.originalScore()
            ));
        }
        return new RerankResponse(items, PROVIDER, 0L);
    }

    @Override
    public String name() {
        return PROVIDER;
    }
}
