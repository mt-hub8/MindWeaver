(function () {
    const SUGGESTED_QUESTIONS = [
        "Embedding Cache 使用什么作为 cache key？",
        "Transactional Outbox 解决了什么问题？",
        "RAG 检索中的 citations 有什么作用？",
        "Hybrid retrieval 与 dense-only 有什么区别？"
    ];

    const queryInput = document.getElementById("query-input");
    const topKInput = document.getElementById("topk-input");
    const qualityModeSelect = document.getElementById("quality-mode-select");
    const scopeSelect = document.getElementById("scope-select");
    const scopeHint = document.getElementById("scope-hint");
    const askButton = document.getElementById("ask-button");
    const loadingStatus = document.getElementById("loading-status");
    const errorStatus = document.getElementById("error-status");
    const errorCode = document.getElementById("error-code");
    const errorMessage = document.getElementById("error-message");
    const errorTraceId = document.getElementById("error-trace-id");
    const suggestionContainer = document.getElementById("suggestion-chips");

    const answerSection = document.getElementById("answer-section");
    const answerScopeLabel = document.getElementById("answer-scope-label");
    const answerSummary = document.getElementById("answer-summary");
    const answerTechDetails = document.getElementById("answer-tech-details");
    const answerContent = document.getElementById("answer-content");
    const citationsSection = document.getElementById("citations-section");
    const citationsEmpty = document.getElementById("citations-empty");
    const citationsList = document.getElementById("citations-list");
    const retrievalSection = document.getElementById("retrieval-section");
    const retrievalSummary = document.getElementById("retrieval-summary");
    const retrievalMetadata = document.getElementById("retrieval-metadata");
    const queryUnderstandingSection = document.getElementById("query-understanding-section");
    const queryUnderstandingSummary = document.getElementById("query-understanding-summary");
    const queryUnderstandingMetadata = document.getElementById("query-understanding-metadata");
    const queryRewriteSection = document.getElementById("query-rewrite-section");
    const queryRewriteSummary = document.getElementById("query-rewrite-summary");
    const generationSection = document.getElementById("generation-section");
    const generationSummary = document.getElementById("generation-summary");
    const generationMetadata = document.getElementById("generation-metadata");

    const qualitySection = document.getElementById("quality-section");
    const qualityOverallScore = document.getElementById("quality-overall-score");
    const qualityOverallLevel = document.getElementById("quality-overall-level");
    const qualityModeLabel = document.getElementById("quality-mode-label");
    const qualityDimensions = document.getElementById("quality-dimensions");
    const qualityIssues = document.getElementById("quality-issues");
    const qualityIssuesEmpty = document.getElementById("quality-issues-empty");
    const qualitySuggestions = document.getElementById("quality-suggestions");
    const qualitySuggestionsEmpty = document.getElementById("quality-suggestions-empty");
    const qualityMetricDetails = document.getElementById("quality-metric-details");

    renderSuggestedQuestions();
    askButton.addEventListener("click", submitQuestion);
    scopeSelect.addEventListener("change", updateScopeHint);
    loadCollections();

    const params = new URLSearchParams(window.location.search);
    const initialCollectionId = params.get("collectionId");
    if (initialCollectionId) {
        scopeSelect.dataset.pendingCollectionId = initialCollectionId;
    }

    function updateScopeHint() {
        const selected = scopeSelect.options[scopeSelect.selectedIndex];
        if (!scopeSelect.value) {
            scopeHint.textContent = "当前回答基于全部已启用文档。已删除文档不会进入回答引用；旧版本片段不会进入回答引用。";
        } else {
            scopeHint.textContent = "仅在该分组中提问：当前回答只基于「" + (selected.textContent || "所选分组") + "」中的文档。";
        }
    }

    async function loadCollections() {
        try {
            const response = await fetch("/collections");
            const payload = await response.json();
            if (!response.ok || !Array.isArray(payload)) {
                return;
            }
            payload.forEach(function (collection) {
                const option = document.createElement("option");
                option.value = String(collection.collectionId);
                option.textContent = collection.name || ("分组 " + collection.collectionId);
                scopeSelect.appendChild(option);
            });
            const pending = scopeSelect.dataset.pendingCollectionId;
            if (pending) {
                scopeSelect.value = pending;
                delete scopeSelect.dataset.pendingCollectionId;
            }
            updateScopeHint();
        } catch (error) {
            // 分组列表加载失败时仍可使用「全部文档」问答
        }
    }

    function renderSuggestedQuestions() {
        if (!suggestionContainer) {
            return;
        }
        SUGGESTED_QUESTIONS.forEach(function (question) {
            const button = document.createElement("button");
            button.type = "button";
            button.className = "chip-button";
            button.textContent = question;
            button.addEventListener("click", function () {
                queryInput.value = question;
                queryInput.focus();
            });
            suggestionContainer.appendChild(button);
        });
    }

    async function submitQuestion() {
        clearError();
        clearResults();

        const query = queryInput.value.trim();
        const topK = parseTopK(topKInput.value);
        const collectionId = scopeSelect.value ? Number.parseInt(scopeSelect.value, 10) : null;

        if (!query) {
            showError({
                code: "CLIENT_VALIDATION",
                message: "问题不能为空。",
                traceId: "-"
            });
            return;
        }

        if (topK === null) {
            showError({
                code: "CLIENT_VALIDATION",
                message: "Top K 必须是 1 到 10 之间的整数。",
                traceId: "-"
            });
            return;
        }

        setLoading(true);

        const requestBody = { query: query, topK: topK };
        if (collectionId) {
            requestBody.collectionId = collectionId;
        }
        if (qualityModeSelect && qualityModeSelect.value) {
            requestBody.qualityMode = qualityModeSelect.value;
        }

        try {
            const response = await fetch("/rag/answers", {
                method: "POST",
                headers: {
                    "Content-Type": "application/json"
                },
                body: JSON.stringify(requestBody)
            });

            const responseText = await response.text();
            let payload = null;
            if (responseText) {
                try {
                    payload = JSON.parse(responseText);
                } catch (parseError) {
                    payload = null;
                }
            }

            if (!response.ok) {
                if (payload && payload.code && payload.message) {
                    showError({
                        code: payload.code,
                        message: payload.message,
                        traceId: payload.traceId || "-"
                    });
                } else {
                    showError({
                        code: "HTTP_" + response.status,
                        message: responseText || "请求失败，未返回标准错误响应。",
                        traceId: "-"
                    });
                }
                return;
            }

            renderSuccess(payload || {}, collectionId);
        } catch (networkError) {
            showError({
                code: "NETWORK_ERROR",
                message: networkError && networkError.message
                    ? networkError.message
                    : "网络请求失败，请确认服务已启动且可访问。",
                traceId: "-"
            });
        } finally {
            setLoading(false);
        }
    }

    function parseTopK(value) {
        const parsed = Number.parseInt(value, 10);
        if (!Number.isInteger(parsed) || parsed < 1 || parsed > 10) {
            return null;
        }
        return parsed;
    }

    function setLoading(isLoading) {
        askButton.disabled = isLoading;
        loadingStatus.classList.toggle("visible", isLoading);
    }

    function clearError() {
        errorStatus.classList.add("hidden");
        errorCode.textContent = "";
        errorMessage.textContent = "";
        errorTraceId.textContent = "";
    }

    function showError(error) {
        errorCode.textContent = "code: " + (error.code || "UNKNOWN");
        errorMessage.textContent = "message: " + (error.message || "未知错误");
        errorTraceId.textContent = "traceId: " + (error.traceId || "-");
        errorStatus.classList.remove("hidden");
    }

    function clearResults() {
        answerSection.classList.add("hidden");
        qualitySection.classList.add("hidden");
        citationsSection.classList.add("hidden");
        retrievalSection.classList.add("hidden");
        queryUnderstandingSection.classList.add("hidden");
        queryRewriteSection.classList.add("hidden");
        generationSection.classList.add("hidden");
        answerContent.textContent = "";
        answerScopeLabel.textContent = "";
        answerScopeLabel.classList.add("hidden");
        answerSummary.innerHTML = "";
        if (answerTechDetails) {
            answerTechDetails.classList.add("hidden");
        }
        citationsList.innerHTML = "";
        citationsEmpty.classList.add("hidden");
        retrievalSummary.innerHTML = "";
        retrievalMetadata.textContent = "";
        queryUnderstandingSummary.innerHTML = "";
        queryUnderstandingMetadata.textContent = "";
        queryRewriteSummary.innerHTML = "";
        generationSummary.innerHTML = "";
        generationMetadata.textContent = "";
        qualityOverallScore.textContent = "-";
        qualityOverallLevel.textContent = "-";
        qualityModeLabel.textContent = "平衡模式";
        setDimensionScore("quality-retrieval-score", null);
        setDimensionScore("quality-context-score", null);
        setDimensionScore("quality-answer-score", null);
        setDimensionScore("quality-citation-score", null);
        qualityIssues.innerHTML = "";
        qualitySuggestions.innerHTML = "";
        qualityIssuesEmpty.classList.add("hidden");
        qualitySuggestionsEmpty.classList.add("hidden");
        qualityMetricDetails.textContent = "";
    }

    function renderSuccess(data, requestedCollectionId) {
        renderQualityScore(data.qualityScore);

        answerSection.classList.remove("hidden");
        answerContent.textContent = data.answer || "（无 answer 字段）";

        const retrieval = data.retrieval || {};
        renderQueryUnderstanding(data.queryUnderstanding || {});
        const scopeLabel = buildScopeLabel(retrieval, requestedCollectionId);
        if (scopeLabel) {
            answerScopeLabel.textContent = scopeLabel;
            answerScopeLabel.classList.remove("hidden");
        }

        const generation = data.generation || {};
        const citations = Array.isArray(data.citations) ? data.citations : [];
        renderAnswerSummary(generation, citations.length);

        citationsSection.classList.remove("hidden");
        if (citations.length === 0) {
            citationsEmpty.classList.remove("hidden");
        } else {
            citationsEmpty.classList.add("hidden");
            citations.forEach(function (citation) {
                citationsList.appendChild(renderCitation(citation));
            });
        }

        retrievalSection.classList.remove("hidden");
        renderSummaryDl(retrievalSummary, buildRetrievalSummary(retrieval));
        retrievalMetadata.textContent = prettyJson(data.retrieval);

        generationSection.classList.remove("hidden");
        renderSummaryDl(generationSummary, buildGenerationSummary(generation));
        generationMetadata.textContent = prettyJson(data.generation);
    }

    function renderQueryUnderstanding(queryUnderstanding) {
        if (!queryUnderstanding || Object.keys(queryUnderstanding).length === 0) {
            return;
        }
        queryUnderstandingSection.classList.remove("hidden");
        renderSummaryDl(queryUnderstandingSummary, [
            ["Query Type", safeValue(queryUnderstanding.queryType)],
            ["Version", safeValue(queryUnderstanding.extractedVersion)],
            ["Doc Type", safeValue(queryUnderstanding.extractedDocType)],
            ["Code Symbol", safeValue((queryUnderstanding.extractedSymbols || []).join(", "))],
            ["Config Key", safeValue((queryUnderstanding.extractedConfigKeys || []).join(", "))],
            ["Confidence", queryUnderstanding.confidence != null ? String(queryUnderstanding.confidence) : "-"]
        ]);
        queryUnderstandingMetadata.textContent = prettyJson(queryUnderstanding);

        const rewritten = queryUnderstanding.rewrittenQueries || {};
        queryRewriteSection.classList.remove("hidden");
        renderSummaryDl(queryRewriteSummary, [
            ["normalizedQuery", safeValue(rewritten.normalizedQuery)],
            ["keywordQuery", safeValue(rewritten.keywordQuery)],
            ["semanticQuery", safeValue(rewritten.semanticQuery)],
            ["symbolQuery", safeValue(rewritten.symbolQuery)]
        ]);
    }

    function renderQualityScore(qualityScore) {
        qualitySection.classList.remove("hidden");
        if (!qualityScore) {
            qualityOverallScore.textContent = "-";
            qualityOverallLevel.textContent = "暂无评分";
            qualityIssuesEmpty.classList.remove("hidden");
            qualitySuggestionsEmpty.classList.remove("hidden");
            return;
        }

        qualityOverallScore.textContent = qualityScore.overallScore != null ? String(qualityScore.overallScore) : "-";
        qualityOverallLevel.textContent = qualityScore.displayLevel || qualityScore.overallLevel || "-";
        qualityModeLabel.textContent = qualityScore.displayMode || "平衡模式";

        setDimensionScore("quality-retrieval-score", qualityScore.retrievalScore);
        setDimensionScore("quality-context-score", qualityScore.contextScore);
        setDimensionScore("quality-answer-score", qualityScore.answerScore);
        setDimensionScore("quality-citation-score", qualityScore.citationScore);

        const issues = Array.isArray(qualityScore.diagnosis && qualityScore.diagnosis.issues)
            ? qualityScore.diagnosis.issues
            : [];
        qualityIssues.innerHTML = "";
        if (issues.length === 0) {
            qualityIssuesEmpty.classList.remove("hidden");
        } else {
            qualityIssuesEmpty.classList.add("hidden");
            issues.forEach(function (issue) {
                const item = document.createElement("li");
                item.textContent = (issue.title || issue.code || "问题") + "：" + (issue.description || "");
                qualityIssues.appendChild(item);
            });
        }

        const suggestions = Array.isArray(qualityScore.diagnosis && qualityScore.diagnosis.suggestions)
            ? qualityScore.diagnosis.suggestions
            : [];
        qualitySuggestions.innerHTML = "";
        if (suggestions.length === 0) {
            qualitySuggestionsEmpty.classList.remove("hidden");
        } else {
            qualitySuggestionsEmpty.classList.add("hidden");
            suggestions.forEach(function (suggestion) {
                const item = document.createElement("li");
                item.textContent = (suggestion.title || suggestion.code || "建议") + "：" + (suggestion.description || "");
                qualitySuggestions.appendChild(item);
            });
        }

        const technicalPayload = {
            summary: qualityScore.diagnosis ? qualityScore.diagnosis.summary : null,
            scoringNote: qualityScore.scoringNote,
            weights: qualityScore.weights,
            metricDetails: qualityScore.metricDetails,
            mode: qualityScore.mode,
            displayMode: qualityScore.displayMode
        };
        qualityMetricDetails.textContent = prettyJson(technicalPayload);
    }

    function formatScore(value) {
        if (value === undefined || value === null) {
            return "-";
        }
        return String(value) + " / 100";
    }

    function setDimensionScore(elementId, value) {
        const element = document.getElementById(elementId);
        if (element) {
            element.textContent = formatScore(value);
        }
    }

    function renderAnswerSummary(generation, citationCount) {
        const items = [];
        if (generation.llmModel || generation.model) {
            items.push(["使用模型", (generation.llmProvider || generation.provider || "-") +
                " / " + (generation.llmModel || generation.model || "-")]);
        }
        if (generation.latencyMs != null) {
            items.push(["耗时", generation.latencyMs + " ms"]);
        }
        if (citationCount != null) {
            items.push(["引用条数", String(citationCount)]);
        }
        if (items.length === 0) {
            return;
        }
        renderSummaryDl(answerSummary, items);
        if (answerTechDetails) {
            answerTechDetails.classList.remove("hidden");
        }
    }

    function buildRetrievalSummary(retrieval) {
        const items = [];
        if (retrieval.returned != null) {
            items.push(["命中片段数", String(retrieval.returned)]);
        }
        if (retrieval.strategy) {
            items.push(["检索策略", retrieval.strategy]);
        }
        if (retrieval.scopeType) {
            items.push(["范围类型", retrieval.scopeType]);
        }
        if (retrieval.denseTopK != null) {
            items.push(["vectorTopK", String(retrieval.denseTopK)]);
        }
        if (retrieval.lexicalTopK != null) {
            items.push(["keywordTopK", String(retrieval.lexicalTopK)]);
        }
        if (retrieval.finalTopK != null) {
            items.push(["finalTopK", String(retrieval.finalTopK)]);
        }
        if (retrieval.rerankEnabled != null) {
            items.push(["rerank", retrieval.rerankEnabled ? "enabled" : "disabled"]);
        }
        if (retrieval.contextExpansion) {
            items.push(["contextExpansion", retrieval.contextExpansion]);
        }
        return items;
    }

    function buildGenerationSummary(generation) {
        const items = [];
        if (generation.llmModel || generation.model) {
            items.push(["模型", generation.llmModel || generation.model]);
        }
        if (generation.latencyMs != null) {
            items.push(["耗时", generation.latencyMs + " ms"]);
        }
        if (generation.inputTokens != null || generation.outputTokens != null) {
            items.push(["Token", (generation.inputTokens || 0) + " / " + (generation.outputTokens || 0)]);
        }
        return items;
    }

    function renderSummaryDl(container, items) {
        container.innerHTML = "";
        items.forEach(function (pair) {
            const dt = document.createElement("dt");
            dt.textContent = pair[0];
            const dd = document.createElement("dd");
            dd.textContent = pair[1];
            container.appendChild(dt);
            container.appendChild(dd);
        });
    }

    function buildScopeLabel(retrieval, requestedCollectionId) {
        if (retrieval.scopeType === "COLLECTION" || requestedCollectionId) {
            const name = retrieval.collectionName
                ? "「" + retrieval.collectionName + "」"
                : "所选分组";
            return "当前问答范围：仅在该分组中提问 · " + name;
        }
        return "当前问答范围：全部文档";
    }

    function renderCitation(citation) {
        const item = document.createElement("li");
        item.className = "citation-item";

        const title = document.createElement("h3");
        title.textContent = "[" + safeValue(citation.sourceIndex) + "] 引用来源";
        item.appendChild(title);

        const meta = document.createElement("div");
        meta.className = "citation-meta";
        meta.innerHTML =
            "<span>相关度: " + safeValue(citation.score) + "</span>";
        item.appendChild(meta);

        const tech = document.createElement("details");
        tech.className = "tech-details";
        tech.innerHTML =
            "<summary>查看技术详情</summary><pre>" +
            escapeHtml(prettyJson(citation)) +
            "</pre>";
        item.appendChild(tech);

        const snippet = document.createElement("p");
        snippet.className = "citation-snippet";
        snippet.textContent = citation.contentSnippet || "（无片段内容）";
        item.appendChild(snippet);

        return item;
    }

    function escapeHtml(value) {
        if (value === undefined || value === null) {
            return "";
        }
        return String(value)
            .replace(/&/g, "&amp;")
            .replace(/</g, "&lt;")
            .replace(/>/g, "&gt;")
            .replace(/"/g, "&quot;");
    }

    function prettyJson(value) {
        if (value === undefined || value === null) {
            return "{}";
        }
        try {
            return JSON.stringify(value, null, 2);
        } catch (error) {
            return String(value);
        }
    }

    function safeValue(value) {
        if (value === undefined || value === null || value === "") {
            return "-";
        }
        return String(value);
    }
})();
