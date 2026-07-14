package com.tuoman.ai_task_orchestrator.evaluation;

import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;

import java.util.ArrayList;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Set;
import java.util.regex.Matcher;
import java.util.regex.Pattern;
import java.util.stream.Collectors;

/**
 * V2.4.4 benchmark evidence 映射器。
 *
 * benchmark corpus 中使用 [EVIDENCE:id] 标记人工证据，再在切分后映射到真实 chunkId。
 * 这样 fixed/adaptive chunking 或不同 VectorStore 可以在同一 Gold Test Set 上比较。
 *
 * 约束：evidence marker 只服务离线评测，不能进入线上 RAG prompt 或业务检索逻辑。
 */
public final class BenchmarkEvidenceMapper {

    private static final Pattern EVIDENCE_MARKER_PATTERN = Pattern.compile("\\[EVIDENCE:([a-zA-Z0-9\\-_]+)]");

    private BenchmarkEvidenceMapper() {
    }

    public static List<Long> mapEvidenceIdsToChunkIds(
            List<String> expectedEvidenceIds,
            List<DocumentChunkEntity> chunks
    ) {
        // 每个 evidence id 必须唯一映射到一个真实 chunk。
        // 多个或零个匹配都说明 benchmark corpus 与当前 chunking 结果不一致，应显式失败。
        if (expectedEvidenceIds == null || expectedEvidenceIds.isEmpty()) {
            throw new IllegalArgumentException("expectedEvidenceIds must not be empty");
        }

        List<Long> expectedChunkIds = new ArrayList<>();
        for (String evidenceId : expectedEvidenceIds) {
            String marker = "[EVIDENCE:" + evidenceId + "]";
            List<DocumentChunkEntity> matchingChunks = chunks.stream()
                    .filter(chunk -> chunk.getContent().contains(marker))
                    .toList();

            if (matchingChunks.size() != 1) {
                throw new IllegalArgumentException(
                        "evidence marker " + marker + " should map to exactly one real chunk, found "
                                + matchingChunks.size()
                );
            }

            expectedChunkIds.add(matchingChunks.getFirst().getId());
        }

        return expectedChunkIds.stream()
                .collect(Collectors.collectingAndThen(
                        Collectors.toCollection(LinkedHashSet::new),
                        List::copyOf
                ));
    }

    public static Set<String> parseEvidenceMarkerIds(String corpus) {
        Matcher matcher = EVIDENCE_MARKER_PATTERN.matcher(corpus);
        Set<String> markerIds = new LinkedHashSet<>();
        while (matcher.find()) {
            String markerId = matcher.group(1);
            if (markerId == null || markerId.isBlank()) {
                throw new IllegalArgumentException("evidence marker id must not be blank");
            }
            if (!markerIds.add(markerId)) {
                throw new IllegalArgumentException("duplicate evidence marker id: " + markerId);
            }
        }
        if (markerIds.isEmpty()) {
            throw new IllegalArgumentException("corpus must contain at least one evidence marker");
        }
        return markerIds;
    }

    public static Set<String> expectedEvidenceIds(RetrievalBenchmarkDataset dataset) {
        return dataset.cases().stream()
                .flatMap(benchmarkCase -> benchmarkCase.expectedEvidenceIds().stream())
                .collect(Collectors.toSet());
    }
}
