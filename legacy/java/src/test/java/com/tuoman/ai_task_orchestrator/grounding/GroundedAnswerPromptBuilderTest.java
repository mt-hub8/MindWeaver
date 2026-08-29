package com.tuoman.ai_task_orchestrator.grounding;

import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

class GroundedAnswerPromptBuilderTest {

    private final GroundedAnswerPromptBuilder builder = new GroundedAnswerPromptBuilder();

    @Test
    void promptShouldContainContractRulesAndCitationFormat() {
        String prompt = builder.buildPrompt(
                "V10.0 的配置是什么？",
                bundle("V10.0 支持 app.llm.provider [source]"),
                new GroundedAnswerContract(AnswerContractMode.STRICT),
                null
        );

        assertThat(prompt).contains("只能使用【资料】中的内容");
        assertThat(prompt).contains("每个关键结论后必须给出引用，例如 [1]");
        assertThat(prompt).contains("STRICT 模式");
        assertThat(prompt).contains("[1]");
        assertThat(prompt).contains("文档：demo.md");
    }

    @Test
    void noContextShouldBuildRefusalPrompt() {
        String prompt = builder.buildPrompt(
                "unknown",
                GroundedContextBundle.builder().contextId("empty").query("unknown").chunks(List.of()).citations(List.of()).build(),
                new GroundedAnswerContract(AnswerContractMode.BALANCED),
                null
        );

        assertThat(prompt).contains("当前没有可用【资料】");
        assertThat(prompt).contains("根据当前资料无法确认");
    }

    private GroundedContextBundle bundle(String text) {
        GroundedContextChunk chunk = GroundedContextChunk.builder()
                .chunkId(101L)
                .documentId(10L)
                .documentTitle("demo.md")
                .sectionPath("配置")
                .version("V10.0")
                .docType("README")
                .text(text)
                .citationKey("[1]")
                .directHit(true)
                .build();
        return GroundedContextBundle.builder()
                .contextId("ctx")
                .query("q")
                .chunks(List.of(chunk))
                .citations(List.of(Citation.builder().citationKey("[1]").chunkId(101L).build()))
                .contextBudget(6000)
                .usedChars(text.length())
                .usedTokensEstimate(10)
                .warnings(List.of())
                .build();
    }
}
