package com.tuoman.ai_task_orchestrator.document;

import com.tuoman.ai_task_orchestrator.config.ChunkingProperties;
import com.tuoman.ai_task_orchestrator.embedding.ChunkHashService;
import com.tuoman.ai_task_orchestrator.enums.ChunkType;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;

import java.util.ArrayList;
import java.util.List;
import java.util.Locale;
import java.util.regex.Pattern;

/**
 * V15.0 引入的结构化 chunking 服务。
 *
 * 它在基础字符切分之上补充 sectionPath、sectionTitle、chunkType、hash、
 * parentChunkIndex 和 previous/next 关系。输出给 DocumentService 持久化，
 * 后续用于 Hybrid Retrieval、metadata pre-filter、context expansion 和 citation 展示。
 *
 * 设计目的：技术文档不是纯长文本，标题、代码块、表格、列表和章节层级会影响检索质量；
 * 结构化 chunk 能让召回和上下文回填更可解释。
 */
@Service
@RequiredArgsConstructor
public class StructuredChunkingService {

    private static final Pattern CODE_FENCE = Pattern.compile("```[\\s\\S]*?```", Pattern.MULTILINE);

    private final DocumentChunker documentChunker;

    private final ChunkingProperties chunkingProperties;

    private final ChunkHashService chunkHashService;

    public List<StructuredChunkResult> chunk(String content) {
        // 阶段 1：先复用基础 chunker 得到稳定的文本片段。
        // 阶段 2：再为每个片段补充结构化 metadata，避免检索层自行猜测章节关系。
        if (content == null || content.isBlank()) {
            return List.of();
        }
        int maxChars = chunkingProperties.getMaxChars();
        int overlap = chunkingProperties.isStrategyWithOverlap() ? chunkingProperties.getOverlapChars() : 0;

        List<DocumentChunkResult> baseChunks = documentChunker.chunk(content, maxChars, overlap);
        List<StructuredChunkResult> results = new ArrayList<>();
        Integer parentIndexForSection = null;
        String currentSection = null;

        for (int i = 0; i < baseChunks.size(); i++) {
            DocumentChunkResult base = baseChunks.get(i);
            String sectionPath = chunkingProperties.isIncludeSectionPath()
                    ? (base.getHeadingPath() != null ? base.getHeadingPath() : inferSectionFromContent(base.getContent()))
                    : base.getHeadingPath();

            if (sectionPath != null && !sectionPath.equals(currentSection)) {
                // 同一 section 内的后续 chunk 记录 parent，用于 parent context expansion。
                currentSection = sectionPath;
                parentIndexForSection = i;
            }

            ChunkType chunkType = detectChunkType(base.getContent());
            String hash = safeHash(base.getContent());
            String normalizedHash = hash;

            results.add(new StructuredChunkResult(
                    base.getChunkIndex(),
                    base.getContent(),
                    base.getContentLength(),
                    resolveStrategyName(),
                    base.getStartOffset(),
                    base.getEndOffset(),
                    base.getHeadingPath(),
                    sectionPath,
                    extractSectionTitle(sectionPath),
                    extractHeadingLevel(base.getContent()),
                    chunkType,
                    hash,
                    normalizedHash,
                    estimateTokenCount(base.getContent()),
                    detectLanguage(base.getContent()),
                    i > 0 ? base.getChunkIndex() - 1 : null,
                    parentIndexForSection != null && parentIndexForSection != i ? parentIndexForSection : null
            ));
        }
        return results;
    }

    private String resolveStrategyName() {
        return chunkingProperties.getStrategy() == null
                ? DocumentChunker.CHUNK_STRATEGY
                : chunkingProperties.getStrategy().name();
    }

    private ChunkType detectChunkType(String content) {
        // chunkType 是检索诊断和未来路由的轻量信号。
        // 例如 CODE_BLOCK / TABLE / LIST 往往比普通 TEXT 更需要 keyword retrieval 保底。
        if (content == null || content.isBlank()) {
            return ChunkType.UNKNOWN;
        }
        String trimmed = content.trim();
        if (trimmed.startsWith("#")) {
            return ChunkType.TITLE;
        }
        if (trimmed.startsWith("```") || CODE_FENCE.matcher(trimmed).find()) {
            return ChunkType.CODE_BLOCK;
        }
        if (trimmed.startsWith(">")) {
            return ChunkType.QUOTE;
        }
        if (trimmed.lines().allMatch(line -> line.isBlank() || line.trim().startsWith("-") || line.trim().matches("^\\d+\\..*"))) {
            return ChunkType.LIST;
        }
        if (trimmed.contains("|") && trimmed.contains("---") && trimmed.lines().filter(l -> l.contains("|")).count() >= 2) {
            return ChunkType.TABLE;
        }
        return ChunkType.TEXT;
    }

    private String inferSectionFromContent(String content) {
        if (content == null) {
            return null;
        }
        for (String line : content.split("\n")) {
            if (line.startsWith("### ")) {
                return line.substring(4).trim();
            }
            if (line.startsWith("## ")) {
                return line.substring(3).trim();
            }
            if (line.startsWith("# ")) {
                return line.substring(2).trim();
            }
        }
        return null;
    }

    private String extractSectionTitle(String sectionPath) {
        if (sectionPath == null || sectionPath.isBlank()) {
            return null;
        }
        int idx = sectionPath.lastIndexOf(" > ");
        return idx < 0 ? sectionPath : sectionPath.substring(idx + 3);
    }

    private Integer extractHeadingLevel(String content) {
        if (content == null) {
            return null;
        }
        String first = content.trim();
        if (first.startsWith("### ")) {
            return 3;
        }
        if (first.startsWith("## ")) {
            return 2;
        }
        if (first.startsWith("# ")) {
            return 1;
        }
        return null;
    }

    private String safeHash(String content) {
        if (content == null || content.isBlank()) {
            return null;
        }
        try {
            return chunkHashService.hash(content);
        } catch (Exception exception) {
            return null;
        }
    }

    private int estimateTokenCount(String content) {
        if (content == null || content.isBlank()) {
            return 0;
        }
        return Math.max(1, content.length() / 4);
    }

    private String detectLanguage(String content) {
        if (content == null || content.isBlank()) {
            return "unknown";
        }
        long cjk = content.chars().filter(ch -> ch >= 0x4E00 && ch <= 0x9FFF).count();
        return cjk > content.length() / 4 ? "zh" : "en";
    }
}
