package com.tuoman.ai_task_orchestrator.embedding;

import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;

class MockEmbeddingClientTest {

    private final MockEmbeddingClient embeddingClient = new MockEmbeddingClient();

    @Test
    void shouldGenerateSameVectorForSameText() {
        EmbeddingResponse first = embed("refund policy");
        EmbeddingResponse second = embed("refund policy");

        assertThat(first.getVector()).containsExactlyElementsOf(second.getVector());
    }

    @Test
    void shouldReturnDefaultMetadata() {
        EmbeddingResponse response = embed("normal text");

        assertThat(response.getProvider()).isEqualTo("mock");
        assertThat(response.getModel()).isEqualTo("mock-embedding-v1");
        assertThat(response.getDimension()).isEqualTo(128);
        assertThat(response.getDistanceMetric()).isEqualTo("COSINE");
    }

    @Test
    void shouldGenerateNonZeroVectorForNonBlankText() {
        EmbeddingResponse response = embed("refund policy");

        assertThat(response.getVector()).hasSize(128);
        assertThat(response.getVector()).anyMatch(value -> value != 0.0);
    }

    @Test
    void shouldGenerateZeroVectorForBlankText() {
        EmbeddingResponse response = embed(" ");

        assertThat(response.getVector()).hasSize(128);
        assertThat(response.getVector()).allMatch(value -> value == 0.0);
    }

    @Test
    void shouldReturnHigherSimilarityForTextsWithSharedKeywords() {
        EmbeddingResponse query = embed("refund policy application");
        EmbeddingResponse related = embed("refund policy should be submitted in seven days");
        EmbeddingResponse unrelated = embed("model router selects mock fast worker");

        double relatedScore = EmbeddingVectorUtils.cosineSimilarity(query.getVector(), related.getVector());
        double unrelatedScore = EmbeddingVectorUtils.cosineSimilarity(query.getVector(), unrelated.getVector());

        assertThat(relatedScore).isGreaterThan(unrelatedScore);
    }

    private EmbeddingResponse embed(String text) {
        EmbeddingRequest request = new EmbeddingRequest();
        request.setText(text);
        return embeddingClient.embed(request);
    }
}
