package com.tuoman.ai_task_orchestrator.hybrid;

import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.context.annotation.Primary;

@Configuration
@EnableConfigurationProperties(RagHybridProperties.class)
public class HybridConfiguration {

    @Bean
    @Primary
    public LexicalRetriever activeLexicalRetriever(SimpleLexicalRetriever simpleLexicalRetriever) {
        return simpleLexicalRetriever;
    }

    @Bean
    @Primary
    public FusionRanker activeFusionRanker(RrfFusionRanker rrfFusionRanker) {
        return rrfFusionRanker;
    }
}
