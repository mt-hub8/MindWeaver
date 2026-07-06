(function () {
    const listEl = document.getElementById("notifications-list");
    const emptyEl = document.getElementById("notifications-empty");
    const unreadEl = document.getElementById("unread-count");
    const refreshBtn = document.getElementById("refresh-notifications-btn");
    const readAllBtn = document.getElementById("read-all-btn");

    refreshBtn.addEventListener("click", loadNotifications);
    readAllBtn.addEventListener("click", markAllRead);
    loadNotifications();
    setInterval(loadUnreadCount, 30000);

    async function loadUnreadCount() {
        try {
            const res = await fetch("/notifications/unread-count");
            if (res.ok) {
                const data = await res.json();
                unreadEl.textContent = data.unreadCount || 0;
            }
        } catch (e) { /* ignore */ }
    }

    async function loadNotifications() {
        await loadUnreadCount();
        try {
            const res = await fetch("/notifications");
            if (!res.ok) return;
            const items = await res.json();
            listEl.innerHTML = "";
            if (!items.length) {
                emptyEl.classList.remove("hidden");
                return;
            }
            emptyEl.classList.add("hidden");
            items.forEach(function (n) {
                const card = document.createElement("article");
                card.className = "entity-card list-card" + (n.status === "UNREAD" ? " unread-notification" : "");
                const targetLink = n.targetType === "UPLOAD_BATCH" && n.targetId
                    ? "/batch-ingestion.html?batchId=" + n.targetId
                    : "#";
                card.innerHTML =
                    "<div class=\"entity-card-header\"><strong>" + escapeHtml(n.title) + "</strong>" +
                    "<span class=\"mw-badge\">" + escapeHtml(n.type) + "</span></div>" +
                    "<p class=\"muted\" style=\"margin:6px 0 0;font-size:0.86rem\">" + escapeHtml(n.message) + "</p>" +
                    "<p class=\"muted\" style=\"margin:4px 0 0;font-size:0.8rem\">" + formatTime(n.createdAt) + "</p>" +
                    "<div class=\"action-row\" style=\"margin-top:8px\">" +
                    (targetLink !== "#" ? "<a href=\"" + targetLink + "\" class=\"mw-button mw-button-secondary secondary-button\" style=\"text-decoration:none\">查看详情</a>" : "") +
                    (n.status === "UNREAD" ? "<button type=\"button\" class=\"mw-button mw-button-secondary secondary-button mark-read-btn\" data-id=\"" + n.id + "\">标记已读</button>" : "") +
                    "</div>";
                listEl.appendChild(card);
            });
            listEl.querySelectorAll(".mark-read-btn").forEach(function (btn) {
                btn.addEventListener("click", function () {
                    markRead(btn.getAttribute("data-id"));
                });
            });
        } catch (e) { /* ignore */ }
    }

    async function markRead(id) {
        await fetch("/notifications/" + id + "/read", { method: "POST" });
        loadNotifications();
    }

    async function markAllRead() {
        await fetch("/notifications/read-all", { method: "POST" });
        loadNotifications();
    }

    function formatTime(iso) {
        if (!iso) return "";
        try { return new Date(iso).toLocaleString("zh-CN"); } catch (e) { return iso; }
    }

    function escapeHtml(text) {
        if (!text) return "";
        return String(text).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
    }
})();
