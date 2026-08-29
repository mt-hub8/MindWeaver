package com.tuoman.ai_task_orchestrator.vectorstore;

import lombok.Getter;
import lombok.Setter;
import org.springframework.boot.context.properties.ConfigurationProperties;

@Getter
@Setter
@ConfigurationProperties(prefix = "app.vector-store")
public class VectorStoreProperties {

    private String provider = ExactCosineVectorStore.PROVIDER;

    private Qdrant qdrant = new Qdrant();

    @Getter
    @Setter
    public static class Qdrant {

        private String baseUrl = "http://127.0.0.1:6333";

        private String collectionName = "ai_task_orchestrator_chunks";

        private String apiKey = "";

        private int timeoutMs = 3000;

        private boolean initializeCollection = false;
    }
}
