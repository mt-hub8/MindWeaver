package com.tuoman.ai_task_orchestrator.document;

import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.List;

@Component
/**
 * V2.1/V2.1.1 早期文档切分器。
 *
 * 先按 Markdown 标题形成 section，再按段落、句子、标点递归拆分，最后退化为固定长度。
 * 输出的 headingPath、offset 和 chunkIndex 是后续 embedding、retrieval evaluation 和 citation 的基础。
 *
 * 约束：chunk 边界会影响召回与引用，切分算法不能为了“凑长度”改变原文顺序或语义。
 */
public class DocumentChunker {

    public static final String CHUNK_STRATEGY = "RECURSIVE_TEXT";

    private static final int DEFAULT_CHUNK_SIZE_CHARS = 1000;

    private static final int DEFAULT_CHUNK_OVERLAP_CHARS = 150;

    public List<DocumentChunkResult> chunk(String content) {
        return chunk(content, DEFAULT_CHUNK_SIZE_CHARS, DEFAULT_CHUNK_OVERLAP_CHARS);
    }

    List<DocumentChunkResult> chunk(String content, int chunkSizeChars, int chunkOverlapChars) {
        if (content == null || content.isBlank()) {
            return List.of();
        }

        // 阶段 1：先保留 Markdown 标题结构。
        // 早期 fixed chunk 只能按长度切，adaptive/recursive chunking 则尽量让证据保持在语义段落内。
        List<TextRange> ranges = new ArrayList<>();
        for (TextRange section : splitByMarkdownHeadings(content)) {
            splitRecursively(content, section.start(), section.end(), section.headingPath(), 0, chunkSizeChars, ranges);
        }

        List<DocumentChunkResult> results = new ArrayList<>();
        int chunkIndex = 0;
        int previousContentEnd = -1;

        for (TextRange range : ranges) {
            // 阶段 2：给相邻 chunk 加 overlap。
            // overlap 改善跨边界问题，但不能被当作新的独立事实来源。
            int contentStart = range.start();
            int contentEnd = range.end();
            String rangeContent = content.substring(contentStart, contentEnd);

            if (rangeContent.isBlank()) {
                continue;
            }

            int startOffset = contentStart;
            if (previousContentEnd >= 0 && chunkOverlapChars > 0) {
                startOffset = Math.max(0, contentStart - chunkOverlapChars);
            }

            String chunkContent = content.substring(startOffset, contentEnd);
            if (chunkContent.isBlank()) {
                continue;
            }

            results.add(new DocumentChunkResult(
                    chunkIndex,
                    chunkContent,
                    chunkContent.length(),
                    CHUNK_STRATEGY,
                    startOffset,
                    contentEnd,
                    range.headingPath()
            ));

            previousContentEnd = contentEnd;
            chunkIndex++;
        }

        return results;
    }

    private List<TextRange> splitByMarkdownHeadings(String content) {
        List<TextRange> sections = new ArrayList<>();
        String[] headings = new String[3];
        int sectionStart = 0;
        String sectionHeadingPath = null;
        int lineStart = 0;

        while (lineStart < content.length()) {
            int lineEnd = content.indexOf('\n', lineStart);
            if (lineEnd < 0) {
                lineEnd = content.length();
            }

            String line = content.substring(lineStart, lineEnd);
            int headingLevel = headingLevel(line);
            if (headingLevel > 0) {
                if (lineStart > sectionStart) {
                    sections.add(new TextRange(sectionStart, lineStart, sectionHeadingPath));
                }

                headings[headingLevel - 1] = line.substring(headingLevel).trim();
                for (int i = headingLevel; i < headings.length; i++) {
                    headings[i] = null;
                }

                sectionStart = lineStart;
                sectionHeadingPath = buildHeadingPath(headings);
            }

            lineStart = lineEnd + 1;
        }

        if (sectionStart < content.length()) {
            sections.add(new TextRange(sectionStart, content.length(), sectionHeadingPath));
        }

        if (sections.isEmpty()) {
            sections.add(new TextRange(0, content.length(), null));
        }

        return sections;
    }

    private int headingLevel(String line) {
        if (line.startsWith("### ")) {
            return 3;
        }
        if (line.startsWith("## ")) {
            return 2;
        }
        if (line.startsWith("# ")) {
            return 1;
        }
        return 0;
    }

    private String buildHeadingPath(String[] headings) {
        List<String> parts = new ArrayList<>();
        for (String heading : headings) {
            if (heading != null && !heading.isBlank()) {
                parts.add(heading);
            }
        }
        return parts.isEmpty() ? null : String.join(" > ", parts);
    }

    private void splitRecursively(
            String content,
            int start,
            int end,
            String headingPath,
            int separatorLevel,
            int chunkSizeChars,
            List<TextRange> ranges
    ) {
        if (end <= start) {
            return;
        }

        if (end - start <= chunkSizeChars) {
            ranges.add(new TextRange(start, end, headingPath));
            return;
        }

        String[] separators = separatorsForLevel(separatorLevel);
        if (separators.length == 0) {
            splitByFixedLength(start, end, headingPath, chunkSizeChars, ranges);
            return;
        }

        List<TextRange> parts = splitBySeparators(content, start, end, headingPath, separators, chunkSizeChars);
        if (parts.isEmpty()) {
            splitRecursively(content, start, end, headingPath, separatorLevel + 1, chunkSizeChars, ranges);
            return;
        }

        for (TextRange part : parts) {
            splitRecursively(content, part.start(), part.end(), headingPath, separatorLevel + 1, chunkSizeChars, ranges);
        }
    }

    private String[] separatorsForLevel(int level) {
        return switch (level) {
            case 0 -> new String[]{"\n\n"};
            case 1 -> new String[]{"\n"};
            case 2 -> new String[]{"。", "？", "！"};
            case 3 -> new String[]{".", "?", "!"};
            case 4 -> new String[]{"，", ",", "；", ";"};
            default -> new String[0];
        };
    }

    private List<TextRange> splitBySeparators(
            String content,
            int start,
            int end,
            String headingPath,
            String[] separators,
            int chunkSizeChars
    ) {
        List<TextRange> parts = new ArrayList<>();
        int currentStart = start;

        while (currentStart < end) {
            int maxEnd = Math.min(end, currentStart + chunkSizeChars);
            int splitEnd = findLastSeparatorEnd(content, currentStart, maxEnd, separators);

            if (splitEnd <= currentStart) {
                return List.of();
            }

            parts.add(new TextRange(currentStart, splitEnd, headingPath));
            currentStart = skipWhitespace(content, splitEnd, end);
        }

        return parts;
    }

    private int findLastSeparatorEnd(String content, int start, int maxEnd, String[] separators) {
        int best = -1;
        for (String separator : separators) {
            int index = content.indexOf(separator, start);
            while (index >= 0 && index + separator.length() <= maxEnd) {
                best = Math.max(best, index + separator.length());
                index = content.indexOf(separator, index + separator.length());
            }
        }
        return best;
    }

    private int skipWhitespace(String content, int start, int end) {
        int index = start;
        while (index < end && Character.isWhitespace(content.charAt(index))) {
            index++;
        }
        return index;
    }

    private void splitByFixedLength(
            int start,
            int end,
            String headingPath,
            int chunkSizeChars,
            List<TextRange> ranges
    ) {
        for (int currentStart = start; currentStart < end; currentStart += chunkSizeChars) {
            ranges.add(new TextRange(
                    currentStart,
                    Math.min(end, currentStart + chunkSizeChars),
                    headingPath
            ));
        }
    }

    private record TextRange(int start, int end, String headingPath) {
    }
}
