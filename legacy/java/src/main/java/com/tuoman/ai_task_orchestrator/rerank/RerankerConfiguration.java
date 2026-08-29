package com.tuoman.ai_task_orchestrator.rerank;

import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.context.annotation.Primary;

import com.tuoman.ai_task_orchestrator.config.RetrievalPipelineProperties;

@Configuration
@EnableConfigurationProperties(RagRerankProperties.class)
public class RerankerConfiguration {

    @Bean
    @Primary
    public Reranker activeReranker(
            RagRerankProperties properties,
            RetrievalPipelineProperties pipelineProperties,
            LexicalOverlapReranker lexicalOverlapReranker,
            SimpleHeuristicReranker simpleHeuristicReranker,
            NoopReranker noopReranker
    ) {
        String provider = properties.getProvider();
        if (pipelineProperties.isHybridEnabled() || "heuristic".equalsIgnoreCase(pipelineProperties.getRerankerMode())) {
            if (SimpleHeuristicReranker.PROVIDER.equalsIgnoreCase(pipelineProperties.getRerankerMode())) {
                return simpleHeuristicReranker;
            }
        }
        if (NoopReranker.PROVIDER.equalsIgnoreCase(provider)) {
            return noopReranker;
        }
        if (SimpleHeuristicReranker.PROVIDER.equalsIgnoreCase(provider)) {
            return simpleHeuristicReranker;
        }
        return lexicalOverlapReranker;
    }
}
