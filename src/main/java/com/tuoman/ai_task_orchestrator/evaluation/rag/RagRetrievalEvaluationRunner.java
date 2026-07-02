package com.tuoman.ai_task_orchestrator.evaluation.rag;

import lombok.extern.slf4j.Slf4j;
import org.springframework.boot.ApplicationArguments;
import org.springframework.boot.ApplicationRunner;
import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.stereotype.Component;

import java.nio.file.Path;

@Slf4j
@Component
@EnableConfigurationProperties(RagRetrievalEvaluationProperties.class)
@ConditionalOnProperty(prefix = "app.evaluation.retrieval", name = "enabled", havingValue = "true")
public class RagRetrievalEvaluationRunner implements ApplicationRunner {

    private final RagRetrievalEvaluationProperties properties;

    private final RagRetrievalEvaluationDatasetLoader datasetLoader;

    private final RagRetrievalEvaluationExecutor evaluationExecutor;

    private final RagRetrievalEvaluationReportWriter reportWriter;

    public RagRetrievalEvaluationRunner(
            RagRetrievalEvaluationProperties properties,
            RagRetrievalEvaluationDatasetLoader datasetLoader,
            RagRetrievalEvaluationExecutor evaluationExecutor,
            RagRetrievalEvaluationReportWriter reportWriter
    ) {
        this.properties = properties;
        this.datasetLoader = datasetLoader;
        this.evaluationExecutor = evaluationExecutor;
        this.reportWriter = reportWriter;
    }

    @Override
    public void run(ApplicationArguments args) throws Exception {
        Path datasetPath = Path.of(properties.getDatasetPath());
        RagRetrievalEvaluationDataset dataset = datasetLoader.load(datasetPath);
        Path outputDir = Path.of(properties.getReportOutputDir());

        if (properties.isCompareHybrid()) {
            RagHybridComparisonReport comparisonReport = evaluationExecutor.evaluateHybridComparison(
                    dataset,
                    datasetPath.toString(),
                    properties.getDefaultTopK(),
                    properties.getDenseTopK(),
                    properties.getLexicalTopK(),
                    properties.getHybridRrfK(),
                    properties.getDocumentId(),
                    properties.isCompareHybridRerank()
            );
            RagRetrievalEvaluationReportWriter.ReportPaths reportPaths = reportWriter.writeHybridComparison(
                    comparisonReport,
                    outputDir
            );
            log.info("RAG hybrid retrieval comparison evaluation done, json={}, markdown={}",
                    reportPaths.jsonPath(), reportPaths.markdownPath());
            return;
        }

        if (properties.isCompareRerank()) {
            RagRetrievalComparisonReport comparisonReport = evaluationExecutor.evaluateComparison(
                    dataset,
                    datasetPath.toString(),
                    properties.getDefaultTopK(),
                    properties.getCandidateTopK(),
                    properties.getDocumentId()
            );
            RagRetrievalEvaluationReportWriter.ReportPaths reportPaths = reportWriter.writeComparison(
                    comparisonReport,
                    outputDir
            );
            log.info("RAG retrieval comparison evaluation done, json={}, markdown={}",
                    reportPaths.jsonPath(), reportPaths.markdownPath());
            return;
        }

        RagRetrievalEvaluationReport report = evaluationExecutor.evaluate(
                dataset,
                datasetPath.toString(),
                properties.getDefaultTopK(),
                properties.getDocumentId()
        );

        RagRetrievalEvaluationReportWriter.ReportPaths reportPaths = reportWriter.write(report, outputDir);
        log.info("RAG retrieval evaluation done, json={}, markdown={}", reportPaths.jsonPath(), reportPaths.markdownPath());
    }
}
