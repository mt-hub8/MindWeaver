(function () {
    const trashEmpty = document.getElementById("trash-empty");
    const trashList = document.getElementById("trash-list");

    const PURGE_CONFIRM =
        "永久删除后将清理原始文件、解析文本、文本片段、向量索引和相关缓存，无法恢复。确定要继续吗？";

    loadTrash();

    async function loadTrash() {
        trashList.innerHTML = "";
        try {
            const response = await fetch("/documents/trash");
            const items = await response.json();
            if (!response.ok || !Array.isArray(items) || items.length === 0) {
                trashEmpty.classList.remove("hidden");
                return;
            }
            trashEmpty.classList.add("hidden");
            items.forEach(function (item) {
                trashList.appendChild(renderCard(item));
            });
        } catch (error) {
            trashEmpty.classList.remove("hidden");
            trashEmpty.querySelector("p").textContent = "加载垃圾箱失败，请确认服务已启动。";
        }
    }

    function renderCard(item) {
        const card = document.createElement("div");
        card.className = "list-card";
        const collections = (item.collectionNames || []).join("、") || "未分组";
        card.innerHTML =
            '<div class="list-card-head">' +
            '<div><h3 class="list-card-title">' + escapeHtml(item.originalFilename || "-") + "</h3>" +
            '<p class="list-card-hint">已放入垃圾箱 · 剩余 ' + escapeHtml(String(item.remainingRetentionDays != null ? item.remainingRetentionDays : "-")) + " 天</p></div>" +
            '<span class="status-pill warning">已放入垃圾箱</span></div>' +
            '<div class="list-card-meta">' +
            "<span>删除时间：<strong>" + escapeHtml(formatDate(item.trashedAt)) + "</strong></span>" +
            "<span>预计永久删除：<strong>" + escapeHtml(formatDate(item.purgeAfter)) + "</strong></span>" +
            "<span>大小：<strong>" + escapeHtml(item.displaySize || "-") + "</strong></span>" +
            "<span>所属分组：<strong>" + escapeHtml(collections) + "</strong></span>" +
            "</div>" +
            '<div class="list-card-actions">' +
            '<button type="button" class="secondary-button restore-btn">恢复</button>' +
            '<button type="button" class="danger-button purge-btn">永久删除</button>' +
            "</div>" +
            '<details class="tech-details"><summary>查看技术详情</summary><pre>' +
            escapeHtml(JSON.stringify({
                documentId: item.documentId,
                status: item.status,
                trashedAt: item.trashedAt,
                purgeAfter: item.purgeAfter
            }, null, 2)) + "</pre></details>";

        card.querySelector(".restore-btn").addEventListener("click", function () {
            restoreDocument(item.documentId);
        });
        card.querySelector(".purge-btn").addEventListener("click", function () {
            purgeDocument(item.documentId);
        });
        return card;
    }

    async function restoreDocument(documentId) {
        try {
            const response = await fetch("/documents/" + documentId + "/restore", { method: "POST" });
            const payload = await response.json();
            if (!response.ok) {
                showInlineError(payload.message || "恢复失败");
                return;
            }
            await loadTrash();
        } catch (e) {
            showInlineError("恢复请求失败");
        }
    }

    async function purgeDocument(documentId) {
        if (!window.confirm(PURGE_CONFIRM)) {
            return;
        }
        try {
            const response = await fetch("/documents/" + documentId + "/purge", { method: "POST" });
            const payload = await response.json();
            if (!response.ok) {
                showInlineError(payload.message || "永久删除失败");
                return;
            }
            await loadTrash();
        } catch (e) {
            showInlineError("永久删除请求失败");
        }
    }

    function showInlineError(msg) {
        var banner = document.createElement("div");
        banner.className = "error-banner";
        banner.innerHTML = "<strong>操作失败</strong><p style='margin:4px 0 0'>" + escapeHtml(msg) + "</p>";
        trashList.parentElement.insertBefore(banner, trashList);
        setTimeout(function () { banner.remove(); }, 5000);
    }

    function formatDate(value) {
        if (!value) return "-";
        try {
            return new Date(value).toLocaleString("zh-CN");
        } catch (e) {
            return String(value);
        }
    }

    function escapeHtml(value) {
        if (value == null) return "";
        return String(value).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
    }
})();
