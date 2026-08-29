package com.tuoman.ai_task_orchestrator.evaluation;

import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import org.junit.jupiter.api.Test;

import java.util.List;
import java.util.Set;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class BenchmarkEvidenceMapperTest {

    @Test
    void mapEvidenceIdsToChunkIdsShouldMapSingleMarkerToChunkId() {
        DocumentChunkEntity chunk = chunk(1L, "answer [EVIDENCE:outbox-reason] text");

        List<Long> chunkIds = BenchmarkEvidenceMapper.mapEvidenceIdsToChunkIds(
                List.of("outbox-reason"),
                List.of(chunk)
        );

        assertThat(chunkIds).containsExactly(1L);
    }

    @Test
    void mapEvidenceIdsToChunkIdsShouldFailWhenMarkerMapsToMultipleChunks() {
        DocumentChunkEntity first = chunk(1L, "[EVIDENCE:dup]");
        DocumentChunkEntity second = chunk(2L, "[EVIDENCE:dup]");

        assertThatThrownBy(() -> BenchmarkEvidenceMapper.mapEvidenceIdsToChunkIds(
                List.of("dup"),
                List.of(first, second)
        ))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("exactly one");
    }

    @Test
    void parseEvidenceMarkerIdsShouldExtractUniqueMarkers() {
        Set<String> markerIds = BenchmarkEvidenceMapper.parseEvidenceMarkerIds(
                "first [EVIDENCE:a] second [EVIDENCE:b]"
        );

        assertThat(markerIds).containsExactlyInAnyOrder("a", "b");
    }

    private DocumentChunkEntity chunk(Long id, String content) {
        DocumentChunkEntity chunk = new DocumentChunkEntity();
        chunk.setId(id);
        chunk.setContent(content);
        return chunk;
    }
}
