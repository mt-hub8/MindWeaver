(function () {
    const loadingStatus = document.getElementById("loading-status");
    const errorStatus = document.getElementById("error-status");
    const errorCode = document.getElementById("error-code");
    const errorMessage = document.getElementById("error-message");
    const errorTraceId = document.getElementById("error-trace-id");
    const emptyState = document.getElementById("documents-empty");
    const documentsPanel = document.getElementById("documents-panel");
    const documentsBody = document.getElementById("documents-body");
    const chunksPanel = document.getElementById("chunks-panel");
    const chunksTitle = document.getElementById("chunks-title");
    const chunksEmpty = document.getElementById("chunks-empty");
    const chunksList = document.getElementById("chunks-list");

    const SNIPPET_MAX = 300;
    let selectedDocumentId = null;

    loadDocuments();

    async function loadDocuments() {
        setLoading(true);
        clearError();
        hideChunks();

        try {
            const response = await fetch("/documents");
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
                showError(extractError(payload, response.status, responseText));
                documentsPanel.classList.add("hidden");
                emptyState.classList.add("hidden");
                return;
            }

            const documents = Array.isArray(payload) ? payload : [];
            renderDocuments(documents);
        } catch (networkError) {
            showError({
                code: "NETWORK_ERROR",
                message: networkError && networkError.message
                    ? networkError.message
                    : "无法加载文档列表，请确认服务已启动。",
                traceId: "-"
            });
            documentsPanel.classList.add("hidden");
            emptyState.classList.add("hidden");
        } finally {
            setLoading(false);
        }
    }

    function renderDocuments(documents) {
        documentsBody.innerHTML = "";
        if (documents.length === 0) {
            documentsPanel.classList.add("hidden");
            emptyState.classList.remove("hidden");
            return;
        }

        emptyState.classList.add("hidden");
        documentsPanel.classList.remove("hidden");

        documents.forEach(function (doc) {
            const row = document.createElement("tr");
            row.className = "doc-row";
            row.dataset.documentId = String(doc.documentId);
            row.innerHTML =
                "<td>" + escapeHtml(doc.documentId) + "</td>" +
                "<td>" + escapeHtml(doc.title || "-") + "</td>" +
                "<td>" + escapeHtml(doc.chunkCount) + "</td>" +
                "<td>" + escapeHtml(doc.status || "-") + "</td>" +
                "<td>" + escapeHtml(formatDate(doc.createdAt)) + "</td>";
            row.addEventListener("click", function () {
                selectDocument(doc.documentId, doc.title, row);
            });
            documentsBody.appendChild(row);
        });
    }

    async function selectDocument(documentId, title, row) {
        selectedDocumentId = documentId;
        Array.from(documentsBody.querySelectorAll(".doc-row")).forEach(function (item) {
            item.classList.toggle("selected", item === row);
        });

        chunksTitle.textContent = "Chunks · " + (title || "Document " + documentId);
        chunksPanel.classList.remove("hidden");
        chunksList.innerHTML = "";
        chunksEmpty.classList.add("hidden");
        setLoading(true);
        clearError();

        try {
            const response = await fetch("/documents/" + documentId + "/chunks");
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
                showError(extractError(payload, response.status, responseText));
                chunksList.innerHTML = "";
                chunksEmpty.classList.remove("hidden");
                chunksEmpty.textContent = "无法加载 chunks。";
                return;
            }

            const chunks = Array.isArray(payload) ? payload : [];
            renderChunks(chunks);
        } catch (networkError) {
            showError({
                code: "NETWORK_ERROR",
                message: networkError && networkError.message
                    ? networkError.message
                    : "无法加载 chunks。",
                traceId: "-"
            });
            chunksEmpty.classList.remove("hidden");
            chunksEmpty.textContent = "无法加载 chunks。";
        } finally {
            setLoading(false);
        }
    }

    function renderChunks(chunks) {
        chunksList.innerHTML = "";
        if (chunks.length === 0) {
            chunksEmpty.classList.remove("hidden");
            chunksEmpty.textContent = "该文档暂无 chunks。";
            return;
        }

        chunksEmpty.classList.add("hidden");
        chunks.forEach(function (chunk) {
            const item = document.createElement("li");
            item.className = "chunk-item";

            const meta = document.createElement("div");
            meta.className = "chunk-meta";
            meta.textContent = "chunkId: " + safeValue(chunk.id) +
                " · index: " + safeValue(chunk.chunkIndex) +
                (chunk.headingPath ? " · " + chunk.headingPath : "");
            item.appendChild(meta);

            const snippet = document.createElement("p");
            snippet.className = "chunk-snippet";
            snippet.textContent = toSnippet(chunk.content);
            item.appendChild(snippet);

            chunksList.appendChild(item);
        });
    }

    function hideChunks() {
        chunksPanel.classList.add("hidden");
        selectedDocumentId = null;
    }

    function setLoading(isLoading) {
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

    function extractError(payload, status, responseText) {
        if (payload && payload.code && payload.message) {
            return {
                code: payload.code,
                message: payload.message,
                traceId: payload.traceId || "-"
            };
        }
        return {
            code: "HTTP_" + status,
            message: responseText || "请求失败。",
            traceId: "-"
        };
    }

    function toSnippet(content) {
        if (!content) {
            return "（无内容）";
        }
        if (content.length <= SNIPPET_MAX) {
            return content;
        }
        return content.substring(0, SNIPPET_MAX) + "…";
    }

    function formatDate(value) {
        if (!value) {
            return "-";
        }
        return String(value).replace("T", " ");
    }

    function safeValue(value) {
        if (value === undefined || value === null || value === "") {
            return "-";
        }
        return String(value);
    }

    function escapeHtml(value) {
        return String(value)
            .replace(/&/g, "&amp;")
            .replace(/</g, "&lt;")
            .replace(/>/g, "&gt;")
            .replace(/"/g, "&quot;");
    }
})();
