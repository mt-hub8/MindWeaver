(function () {
    const trashEmpty = document.getElementById("trash-empty");
    const trashTable = document.getElementById("trash-table");
    const trashBody = document.getElementById("trash-body");

    const PURGE_CONFIRM_MESSAGE =
        "永久删除后将清理原始文件、解析文本、文本片段、向量索引和相关缓存，无法恢复。确定要继续吗？";

    loadTrash();

    async function loadTrash() {
        trashBody.innerHTML = "";
        try {
            const response = await fetch("/documents/trash");
            const items = await response.json();
            if (!response.ok || !Array.isArray(items) || items.length === 0) {
                trashEmpty.classList.remove("hidden");
                trashTable.classList.add("hidden");
                return;
            }
            trashEmpty.classList.add("hidden");
            trashTable.classList.remove("hidden");
            items.forEach(function (item) {
                trashBody.appendChild(renderRow(item));
            });
        } catch (error) {
            trashEmpty.textContent = "加载垃圾箱失败，请确认服务已启动。";
            trashEmpty.classList.remove("hidden");
        }
    }

    function renderRow(item) {
        const row = document.createElement("tr");

        row.appendChild(cell(item.originalFilename || "-"));
        row.appendChild(cell(formatDate(item.trashedAt)));
        row.appendChild(cell(formatDate(item.purgeAfter)));
        row.appendChild(cell(item.remainingRetentionDays != null ? item.remainingRetentionDays + " 天" : "-"));
        row.appendChild(cell(item.displaySize || "-"));
        row.appendChild(cell((item.collectionNames || []).join("、") || "-"));
        row.appendChild(cell(item.displayStatus || "已放入垃圾箱"));

        const actions = document.createElement("td");
        actions.className = "trash-actions";

        const restoreBtn = document.createElement("button");
        restoreBtn.type = "button";
        restoreBtn.className = "restore-btn";
        restoreBtn.textContent = "恢复";
        restoreBtn.addEventListener("click", function () {
            restoreDocument(item.documentId);
        });

        const purgeBtn = document.createElement("button");
        purgeBtn.type = "button";
        purgeBtn.className = "purge-btn";
        purgeBtn.textContent = "永久删除";
        purgeBtn.addEventListener("click", function () {
            purgeDocument(item.documentId);
        });

        const details = document.createElement("details");
        details.className = "tech-details";
        details.innerHTML = "<summary>查看技术详情</summary>";
        const pre = document.createElement("pre");
        pre.textContent = prettyJson({
            documentId: item.documentId,
            status: item.status,
            trashedAt: item.trashedAt,
            purgeAfter: item.purgeAfter,
            canRestore: item.canRestore,
            canPurgeNow: item.canPurgeNow
        });
        details.appendChild(pre);

        actions.appendChild(restoreBtn);
        actions.appendChild(purgeBtn);
        actions.appendChild(details);
        row.appendChild(actions);

        return row;
    }

    async function restoreDocument(documentId) {
        try {
            const response = await fetch("/documents/" + documentId + "/restore", { method: "POST" });
            const payload = await response.json();
            if (!response.ok) {
                alert(payload.message || "恢复失败");
                return;
            }
            alert(payload.message || "文档已恢复");
            loadTrash();
        } catch (error) {
            alert("恢复请求失败");
        }
    }

    async function purgeDocument(documentId) {
        if (!window.confirm(PURGE_CONFIRM_MESSAGE)) {
            return;
        }
        try {
            const response = await fetch("/documents/" + documentId + "/purge", { method: "POST" });
            const payload = await response.json();
            if (!response.ok) {
                alert(payload.message || "永久删除失败");
                return;
            }
            alert(payload.message || "文档已永久删除");
            loadTrash();
        } catch (error) {
            alert("永久删除请求失败");
        }
    }

    function cell(text) {
        const td = document.createElement("td");
        td.textContent = text;
        return td;
    }

    function formatDate(value) {
        if (!value) {
            return "-";
        }
        return String(value).replace("T", " ").substring(0, 16);
    }

    function prettyJson(value) {
        try {
            return JSON.stringify(value, null, 2);
        } catch (error) {
            return "{}";
        }
    }
})();
