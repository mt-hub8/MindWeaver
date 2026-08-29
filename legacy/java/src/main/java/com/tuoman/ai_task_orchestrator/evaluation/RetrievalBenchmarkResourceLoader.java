package com.tuoman.ai_task_orchestrator.evaluation;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.springframework.stereotype.Component;

import java.io.IOException;
import java.net.URISyntaxException;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;

@Component
public class RetrievalBenchmarkResourceLoader {

    private final ObjectMapper objectMapper = new ObjectMapper();

    public RetrievalBenchmarkDataset loadBenchmark(String resource) throws IOException, URISyntaxException {
        return objectMapper.readValue(readResource(resource), RetrievalBenchmarkDataset.class);
    }

    public String readResource(String resource) throws IOException, URISyntaxException {
        return Files.readString(resourcePath(resource), StandardCharsets.UTF_8);
    }

    private Path resourcePath(String resource) throws URISyntaxException {
        URL url = Thread.currentThread().getContextClassLoader().getResource(resource);
        if (url == null) {
            throw new IllegalArgumentException("resource does not exist: " + resource);
        }
        return Path.of(url.toURI());
    }
}
