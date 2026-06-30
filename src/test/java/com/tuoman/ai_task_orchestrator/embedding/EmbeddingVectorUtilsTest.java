package com.tuoman.ai_task_orchestrator.embedding;

import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class EmbeddingVectorUtilsTest {

    @Test
    void shouldSerializeAndDeserializeVector() {
        List<Double> vector = List.of(1.0, 2.5, -3.0);

        String serialized = EmbeddingVectorUtils.serialize(vector);
        List<Double> deserialized = EmbeddingVectorUtils.deserialize(serialized);

        assertThat(deserialized).containsExactly(1.0, 2.5, -3.0);
    }

    @Test
    void shouldReturnCosineCloseToOneForSameNonZeroVector() {
        List<Double> vector = EmbeddingVectorUtils.l2Normalize(List.of(1.0, 2.0, 3.0));

        assertThat(EmbeddingVectorUtils.cosineSimilarity(vector, vector)).isCloseTo(1.0, within(0.000001));
    }

    @Test
    void shouldReturnZeroForZeroVector() {
        assertThat(EmbeddingVectorUtils.cosineSimilarity(
                List.of(0.0, 0.0),
                List.of(1.0, 2.0)
        )).isEqualTo(0.0);
    }

    @Test
    void shouldThrowWhenDimensionsDoNotMatch() {
        assertThatThrownBy(() -> EmbeddingVectorUtils.cosineSimilarity(
                List.of(1.0, 2.0),
                List.of(1.0)
        )).isInstanceOf(IllegalArgumentException.class);
    }

    @Test
    void shouldReturnLowSimilarityForUnrelatedVectors() {
        assertThat(EmbeddingVectorUtils.cosineSimilarity(
                List.of(1.0, 0.0),
                List.of(0.0, 1.0)
        )).isEqualTo(0.0);
    }

    private org.assertj.core.data.Offset<Double> within(double value) {
        return org.assertj.core.data.Offset.offset(value);
    }
}
