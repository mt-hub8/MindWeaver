(function () {
    const nameInput = document.getElementById("collection-name-input");
    const descriptionInput = document.getElementById("collection-description-input");
    const createButton = document.getElementById("create-collection-button");
    const refreshButton = document.getElementById("refresh-collections-button");
    const createLoading = document.getElementById("create-loading");
    const createSuccess = document.getElementById("create-success");
    const createSuccessMessage = document.getElementById("create-success-message");

    const loadingStatus = document.getElementById("loading-status");
    const errorStatus = document.getElementById("error-status");
    const errorCode = document.getElementById("error-code");
    const errorMessage = document.getElementById("error-message");
    const errorTraceId = document.getElementById("error-trace-id");

    const collectionsEmpty = document.getElementById("collections-empty");
    const collectionsPanel = document.getElementById("collections-panel");
    const collectionsBody = document.getElementById("collections-body");

    const detailPanel = document.getElementById("detail-panel");
    const detailTitle = document.getElementById("detail-title");
    const detailDescription = document.getElementById("detail-description");
    const detailDocumentCount = document.getElementById("detail-document-count");
    const detailActiveCount = document.getElementById("detail-active-count");
    const detailEmptyHint = document.getElementById("detail-empty-hint");
    const detailNoAskableHint = document.getElementById("detail-no-askable-hint");
    const detailDocumentsBody = document.getElementById("detail-documents-body");
    const detailTechJson = document.getElementById("detail-tech-json");
    const askInCollectionLink = document.getElementById("ask-in-collection-link");
    const closeDetailButton = document.getElementById("close-detail-button");

    let cachedCollections = [];

    createButton.addEventListener("click", createCollection);
    refreshButton.addEventListener("click", loadCollections);
    closeDetailButton.addEventListener("click", hideDetail);

    const params = new URLSearchParams(window.location.search);
    const initialCollectionId = params.get("collectionId");
    loadCollections().then(function () {
        if (initialCollectionId) {
            showCollectionDetail(initialCollectionId);
        }
    });

    async function loadCollections() {
        setListLoading(true);
        clearError();
        try {
            const response = await fetch("/collections");
            const payload = await parseJsonResponse(response);
            if (!response.ok) {
                showError(extractError(payload, response.status));
                return;
            }
            cachedCollections = Array.isArray(payload) ? payload : [];
            renderCollections(cachedCollections);
        } catch (networkError) {
            showError({
                code: "NETWORK_ERROR",
                message: networkError && networkError.message ? networkError.message : "无法加载分组列表。",
                traceId: "-"
            });
        } finally {
            setListLoading(false);
        }
    }

    async function createCollection() {
        clearError();
        createSuccess.classList.add("hidden");
        const name = nameInput.value.trim();
        if (!name) {
            showError({ code: "CLIENT_VALIDATION", message: "分组名称不能为空。", traceId: "-" });
            return;
        }

        setCreateLoading(true);
        try {
            const response = await fetch("/collections", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({
                    name: name,
                    description: descriptionInput.value.trim() || null
                })
            });
            const payload = await parseJsonResponse(response);
            if (!response.ok) {
                showError(extractError(payload, response.status));
                return;
            }
            createSuccessMessage.textContent = "分组「" + (payload.name || name) + "」创建成功。";
            createSuccess.classList.remove("hidden");
            nameInput.value = "";
            descriptionInput.value = "";
            await loadCollections();
            if (payload.collectionId) {
                showCollectionDetail(payload.collectionId);
            }
        } catch (networkError) {
            showError({
                code: "NETWORK_ERROR",
                message: networkError && networkError.message ? networkError.message : "创建失败，请稍后重试。",
                traceId: "-"
            });
        } finally {
            setCreateLoading(false);
        }
    }

    function renderCollections(collections) {
        collectionsBody.innerHTML = "";
        if (collections.length === 0) {
            collectionsPanel.classList.add("hidden");
            detailPanel.classList.add("hidden");
            collectionsEmpty.classList.remove("hidden");
            return;
        }
        collectionsEmpty.classList.add("hidden");
        collectionsPanel.classList.remove("hidden");

        collections.forEach(function (collection) {
            const row = document.createElement("tr");
            const activeCount = collection.activeDocumentCount != null ? collection.activeDocumentCount : 0;
            const docCount = collection.documentCount != null ? collection.documentCount : 0;
            row.innerHTML =
                "<td><strong>" + escapeHtml(collection.name) + "</strong></td>" +
                "<td>" + escapeHtml(collection.description || "—") + "</td>" +
                "<td>" + escapeHtml(docCount) + "</td>" +
                "<td>" + escapeHtml(activeCount) + "</td>" +
                '<td class="task-actions">' +
                '<button type="button" class="link-button view-detail-button" data-collection-id="'
                + escapeHtml(collection.collectionId) + '">查看分组详情</button> ' +
                '<a class="ask-link" href="/ask.html?collectionId=' + encodeURIComponent(collection.collectionId) + '">前往该分组问答</a>' +
                "</td>";
            row.querySelector(".view-detail-button").addEventListener("click", function () {
                showCollectionDetail(collection.collectionId);
            });
            collectionsBody.appendChild(row);
        });
    }

    async function showCollectionDetail(collectionId) {
        clearError();
        setListLoading(true);
        try {
            const response = await fetch("/collections/" + encodeURIComponent(collectionId));
            const payload = await parseJsonResponse(response);
            if (!response.ok) {
                showError(extractError(payload, response.status));
                return;
            }
            renderDetail(payload);
        } catch (networkError) {
            showError({
                code: "NETWORK_ERROR",
                message: networkError && networkError.message ? networkError.message : "无法加载分组详情。",
                traceId: "-"
            });
        } finally {
            setListLoading(false);
        }
    }

    function renderDetail(detail) {
        collectionsPanel.classList.add("hidden");
        detailPanel.classList.remove("hidden");
        detailTitle.textContent = "分组详情 · " + (detail.name || "未命名");
        detailDescription.textContent = detail.description || "暂无分组说明。";
        detailDocumentCount.textContent = detail.documentCount != null ? detail.documentCount : 0;
        detailActiveCount.textContent = detail.activeDocumentCount != null ? detail.activeDocumentCount : 0;
        askInCollectionLink.href = "/ask.html?collectionId=" + encodeURIComponent(detail.collectionId);
        detailTechJson.textContent = prettyJson(detail);

        const documents = Array.isArray(detail.documents) ? detail.documents : [];
        detailEmptyHint.classList.toggle("hidden", documents.length > 0);
        const askableCount = documents.filter(function (doc) { return doc.canAsk === true; }).length;
        detailNoAskableHint.classList.toggle("hidden", askableCount > 0 || documents.length === 0);

        detailDocumentsBody.innerHTML = "";
        documents.forEach(function (doc) {
            const row = document.createElement("tr");
            const lifecycleClass = doc.status === "DELETED" ? "deleted" : "active";
            row.innerHTML =
                "<td>" + escapeHtml(doc.title || doc.filename || "-") + "</td>" +
                '<td><span class="status-badge ' + lifecycleClass + '">' + escapeHtml(doc.displayStatus || doc.status) + "</span></td>" +
                "<td>" + (doc.canAsk ? '<span class="status-badge ready">是</span>' : '<span class="muted">否</span>') + "</td>" +
                '<td class="task-actions">' +
                (doc.canRemoveFromCollection !== false
                    ? '<button type="button" class="link-button remove-button" data-document-id="'
                    + escapeHtml(doc.documentId) + '">移出分组</button>'
                    : "") +
                "</td>";
            const removeButton = row.querySelector(".remove-button");
            if (removeButton) {
                removeButton.addEventListener("click", function () {
                    confirmRemoveFromCollection(detail.collectionId, doc.documentId, doc.title || doc.filename);
                });
            }
            detailDocumentsBody.appendChild(row);
        });
    }

    function hideDetail() {
        detailPanel.classList.add("hidden");
        collectionsPanel.classList.remove("hidden");
    }

    async function confirmRemoveFromCollection(collectionId, documentId, filename) {
        const confirmed = window.confirm(
            "确认将文档「" + (filename || documentId) + "」移出此分组？\n移出后该文档仍保留在知识库中。"
        );
        if (!confirmed) {
            return;
        }
        clearError();
        try {
            const response = await fetch(
                "/collections/" + encodeURIComponent(collectionId) + "/documents/" + encodeURIComponent(documentId),
                { method: "DELETE" }
            );
            const payload = await parseJsonResponse(response);
            if (!response.ok) {
                showError(extractError(payload, response.status));
                return;
            }
            await showCollectionDetail(collectionId);
            await loadCollections();
        } catch (networkError) {
            showError({
                code: "NETWORK_ERROR",
                message: networkError && networkError.message ? networkError.message : "移出分组失败。",
                traceId: "-"
            });
        }
    }

    function setListLoading(isLoading) {
        loadingStatus.classList.toggle("hidden", !isLoading);
        loadingStatus.classList.toggle("visible", isLoading);
    }

    function setCreateLoading(isLoading) {
        createButton.disabled = isLoading;
        createLoading.classList.toggle("hidden", !isLoading);
    }

    function clearError() {
        errorStatus.classList.remove("visible");
        errorCode.textContent = "";
        errorMessage.textContent = "";
        errorTraceId.textContent = "";
    }

    function showError(error) {
        errorCode.textContent = error.code || "UNKNOWN";
        errorMessage.textContent = error.message || "未知错误";
        errorTraceId.textContent = error.traceId || "-";
        errorStatus.classList.add("visible");
    }

    async function parseJsonResponse(response) {
        const text = await response.text();
        if (!text) {
            return null;
        }
        try {
            return JSON.parse(text);
        } catch (parseError) {
            return null;
        }
    }

    function extractError(payload, status) {
        if (payload && payload.code) {
            return {
                code: payload.code,
                message: payload.message || "请求失败",
                traceId: payload.traceId || "-"
            };
        }
        return {
            code: "HTTP_" + status,
            message: "请求失败（HTTP " + status + "）",
            traceId: "-"
        };
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
        try {
            return JSON.stringify(value, null, 2);
        } catch (error) {
            return String(value);
        }
    }
})();
