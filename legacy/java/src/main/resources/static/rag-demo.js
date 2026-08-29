(function () {
    const queryInput = document.getElementById("query-input");
    const topKInput = document.getElementById("topk-input");
    const askButton = document.getElementById("ask-button");
    const loadingStatus = document.getElementById("loading-status");
    const errorStatus = document.getElementById("error-status");
    const errorCode = document.getElementById("error-code");
    const errorMessage = document.getElementById("error-message");
    const errorTraceId = document.getElementById("error-trace-id");

    const answerSection = document.getElementById("answer-section");
    const answerContent = document.getElementById("answer-content");
    const citationsSection = document.getElementById("citations-section");
    const citationsEmpty = document.getElementById("citations-empty");
    const citationsList = document.getElementById("citations-list");
    const retrievalSection = document.getElementById("retrieval-section");
    const retrievalMetadata = document.getElementById("retrieval-metadata");
    const generationSection = document.getElementById("generation-section");
    const generationMetadata = document.getElementById("generation-metadata");

    askButton.addEventListener("click", submitQuestion);

    async function submitQuestion() {
        clearError();
        clearResults();

        const query = queryInput.value.trim();
        const topK = parseTopK(topKInput.value);

        if (!query) {
            showError({
                code: "CLIENT_VALIDATION",
                message: "Query 不能为空。",
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

        try {
            const response = await fetch("/rag/answers", {
                method: "POST",
                headers: {
                    "Content-Type": "application/json"
                },
                body: JSON.stringify({
                    query: query,
                    topK: topK
                })
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

            renderSuccess(payload || {});
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
        errorStatus.classList.remove("visible");
        errorCode.textContent = "";
        errorMessage.textContent = "";
        errorTraceId.textContent = "";
    }

    function showError(error) {
        errorCode.textContent = "code: " + (error.code || "UNKNOWN");
        errorMessage.textContent = "message: " + (error.message || "未知错误");
        errorTraceId.textContent = "traceId: " + (error.traceId || "-");
        errorStatus.classList.add("visible");
    }

    function clearResults() {
        answerSection.classList.add("hidden");
        citationsSection.classList.add("hidden");
        retrievalSection.classList.add("hidden");
        generationSection.classList.add("hidden");
        answerContent.textContent = "";
        citationsList.innerHTML = "";
        citationsEmpty.classList.add("hidden");
        retrievalMetadata.textContent = "";
        generationMetadata.textContent = "";
    }

    function renderSuccess(data) {
        answerSection.classList.remove("hidden");
        answerContent.textContent = data.answer || "（无 answer 字段）";

        citationsSection.classList.remove("hidden");
        const citations = Array.isArray(data.citations) ? data.citations : [];
        if (citations.length === 0) {
            citationsEmpty.classList.remove("hidden");
        } else {
            citationsEmpty.classList.add("hidden");
            citations.forEach(function (citation) {
                citationsList.appendChild(renderCitation(citation));
            });
        }

        retrievalSection.classList.remove("hidden");
        retrievalMetadata.textContent = prettyJson(data.retrieval);

        generationSection.classList.remove("hidden");
        generationMetadata.textContent = prettyJson(data.generation);
    }

    function renderCitation(citation) {
        const item = document.createElement("li");
        item.className = "citation-item";

        const title = document.createElement("h3");
        title.textContent = "[" + safeValue(citation.sourceIndex) + "] Citation";
        item.appendChild(title);

        const meta = document.createElement("div");
        meta.className = "citation-meta";
        meta.innerHTML =
            "<span>sourceIndex: " + safeValue(citation.sourceIndex) + "</span>" +
            "<span>documentId: " + safeValue(citation.documentId) + "</span>" +
            "<span>chunkId: " + safeValue(citation.chunkId) + "</span>" +
            "<span>score: " + safeValue(citation.score) + "</span>";
        item.appendChild(meta);

        const snippet = document.createElement("p");
        snippet.className = "citation-snippet";
        snippet.textContent = citation.contentSnippet || "（无 contentSnippet）";
        item.appendChild(snippet);

        return item;
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
