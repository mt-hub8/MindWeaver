package com.tuoman.ai_task_orchestrator.embedding;

import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

@SpringBootTest
class ChunkHashServiceTest {

    @Autowired
    private ChunkHashService chunkHashService;

    @Test
    void shouldReturnSameHashForSameText() {
        String hash1 = chunkHashService.hash("transactional outbox reliable dispatch");
        String hash2 = chunkHashService.hash("transactional outbox reliable dispatch");

        assertThat(hash1).isEqualTo(hash2);
        assertThat(hash1).hasSize(64);
    }

    @Test
    void shouldReturnDifferentHashForDifferentText() {
        String hash1 = chunkHashService.hash("transactional outbox");
        String hash2 = chunkHashService.hash("atomic task claim");

        assertThat(hash1).isNotEqualTo(hash2);
    }

    @Test
    void shouldNormalizeCarriageReturnLineFeedBeforeHashing() {
        String unix = chunkHashService.hash("line one\nline two");
        String windows = chunkHashService.hash("line one\r\nline two");

        assertThat(unix).isEqualTo(windows);
    }

    @Test
    void shouldTrimContentBeforeHashing() {
        String trimmed = chunkHashService.hash("chunk content");
        String padded = chunkHashService.hash("  chunk content  ");

        assertThat(trimmed).isEqualTo(padded);
    }

    @Test
    void shouldRejectNullContent() {
        assertThatThrownBy(() -> chunkHashService.hash(null))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("null");
    }

    @Test
    void shouldRejectBlankContent() {
        assertThatThrownBy(() -> chunkHashService.hash("   "))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("blank");
    }
}
