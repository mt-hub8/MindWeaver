(function () {
    const filesInput = document.getElementById("batch-files");
    const batchNameInput = document.getElementById("batch-name");
    const collectionSelect = document.getElementById("batch-collection");
    const duplicatePolicySelect = document.getElementById("duplicate-policy");
    const startBtn = document.getElementById("start-batch-btn");
    const uploadStatus = document.getElementById("upload-status");
    const progressCard = document.getElementById("batch-progress-card");
    const itemsCard = document.getElementById("batch-items-card");
    const itemsList = document.getElementById("batch-items-list");
    const techJson = document.getElementById("batch-tech-json");
    const retryBtn = document.getElementById("retry-failed-btn");
    const cancelBtn = document.getElementById("cancel-batch-btn");

    let currentBatchId = null;
    let pollTimer = null;

    const params = new URLSearchParams(window.location.search);
    if (params.get("batchId")) {
        currentBatchId = params.get("batchId");
        showBatch(currentBatchId);
        startPolling();
    }

    loadCollections();
    startBtn.addEventListener("click", startBatchUpload);
    retryBtn.addEventListener("click", retryFailed);
    cancelBtn.addEventListener("click", cancelBatch);

    async function loadCollections() {
        try {
            const response = await fetch("/collections");
            if (!response.ok) return;
            const collections = await response.json();
            collections.forEach(function (c) {
                const opt = document.createElement("option");
                opt.value = c.id;
                opt.textContent = c.name;
                collectionSelect.appendChild(opt);
            });
        } catch (e) { /* ignore */ }
    }

    async function startBatchUpload() {
        const files = filesInput.files;
        if (!files || files.length === 0) {
            alert("请至少选择一个文件");
            return;
        }
        const formData = new FormData();
        for (let i = 0; i < files.length; i++) {
            formData.append("files", files[i]);
        }
        if (batchNameInput.value.trim()) {
            formData.append("batchName", batchNameInput.value.trim());
        }
        if (collectionSelect.value) {
            formData.append("collectionId", collectionSelect.value);
        }
        formData.append("duplicatePolicy", duplicatePolicySelect.value);

        uploadStatus.classList.remove("hidden");
        startBtn.disabled = true;
        try {
            const response = await fetch("/documents/batches/upload", { method: "POST", body: formData });
            const data = await response.json();
            if (!response.ok) {
                alert(data.message || "批量导入创建失败");
                return;
            }
            currentBatchId = data.batchId;
            const url = new URL(window.location.href);
            url.searchParams.set("batchId", currentBatchId);
            window.history.replaceState({}, "", url);
            showBatch(currentBatchId);
            startPolling();
        } catch (e) {
            alert("网络错误，请稍后重试");
        } finally {
            uploadStatus.classList.add("hidden");
            startBtn.disabled = false;
        }
    }

    function startPolling() {
        if (pollTimer) clearInterval(pollTimer);
        pollTimer = setInterval(function () {
            if (currentBatchId) refreshBatch(currentBatchId);
        }, 3000);
    }

    async function showBatch(batchId) {
        progressCard.classList.remove("hidden");
        itemsCard.classList.remove("hidden");
        await refreshBatch(batchId);
    }

    async function refreshBatch(batchId) {
        try {
            const [detailRes, itemsRes] = await Promise.all([
                fetch("/documents/batches/" + batchId),
                fetch("/documents/batches/" + batchId + "/items")
            ]);
            if (!detailRes.ok) return;
            const detail = await detailRes.json();
            const items = itemsRes.ok ? await itemsRes.json() : [];
            renderSummary(detail.batch);
            renderItems(detail.items || items);
            techJson.textContent = JSON.stringify({ batch: detail, items: items }, null, 2);
            const status = (detail.batch || detail.summary || detail).status;
            if (["COMPLETED", "PARTIAL_FAILED", "FAILED", "CANCELED"].indexOf(status) >= 0 && pollTimer) {
                clearInterval(pollTimer);
                pollTimer = null;
            }
        } catch (e) { /* ignore */ }
    }

    function renderSummary(batch) {
        document.getElementById("stat-total").textContent = batch.totalCount || 0;
        document.getElementById("stat-completed").textContent = batch.completedCount || 0;
        document.getElementById("stat-failed").textContent = batch.failedCount || 0;
        document.getElementById("stat-skipped").textContent = batch.skippedCount || 0;
        document.getElementById("stat-processing").textContent = (batch.processingCount || 0) + (batch.queuedCount || 0) + (batch.pendingCount || 0);
        document.getElementById("stat-canceled").textContent = batch.canceledCount || 0;
        document.getElementById("batch-progress-percent").textContent = (batch.progressPercent || 0) + "%";
        document.getElementById("batch-status-label").textContent = batch.displayStatus || batch.status || "—";
        document.getElementById("batch-summary-message").textContent = batch.summaryMessage || "";
    }

    function renderItems(items) {
        itemsList.innerHTML = "";
        items.forEach(function (item) {
            const card = document.createElement("article");
            card.className = "entity-card list-card";
            card.innerHTML =
                "<div class=\"entity-card-header\"><strong>" + escapeHtml(item.originalFilename) + "</strong>" +
                "<span class=\"mw-badge\">" + escapeHtml(item.displayStatus || item.status) + "</span></div>" +
                "<p class=\"muted\" style=\"margin:6px 0 0;font-size:0.86rem\">" +
                formatSize(item.fileSize) +
                (item.failureMessage ? " · 失败：" + escapeHtml(item.failureMessage) : "") +
                (item.skipReason ? " · " + escapeHtml(item.skipReason) : "") +
                (item.documentId ? " · 文档 #" + item.documentId : "") +
                "</p>" +
                (item.status === "FAILED" ? "<button type=\"button\" class=\"mw-button mw-button-secondary secondary-button item-retry-btn\" data-item-id=\"" + item.id + "\" style=\"margin-top:8px\">重试</button>" : "");
            itemsList.appendChild(card);
        });
        itemsList.querySelectorAll(".item-retry-btn").forEach(function (btn) {
            btn.addEventListener("click", retryFailed);
        });
    }

    async function retryFailed() {
        if (!currentBatchId) return;
        await fetch("/documents/batches/" + currentBatchId + "/retry-failed", { method: "POST" });
        startPolling();
        refreshBatch(currentBatchId);
    }

    async function cancelBatch() {
        if (!currentBatchId) return;
        if (!confirm("确定取消剩余任务？已成功导入的文档不会被删除。")) return;
        await fetch("/documents/batches/" + currentBatchId + "/cancel", { method: "POST" });
        refreshBatch(currentBatchId);
    }

    function formatSize(bytes) {
        if (!bytes) return "—";
        if (bytes < 1024) return bytes + " B";
        return (bytes / 1024).toFixed(1) + " KB";
    }

    function escapeHtml(text) {
        if (!text) return "";
        return String(text).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
    }
})();
