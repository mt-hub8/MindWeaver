(function () {
    var runtimeMode = document.getElementById("dash-runtime-mode");
    var llm = document.getElementById("dash-llm");
    var documents = document.getElementById("dash-documents");
    var agentTask = document.getElementById("dash-agent-task");
    var collections = document.getElementById("dash-collections");
    var recentDocuments = document.getElementById("recent-documents");
    var recentActivity = document.getElementById("recent-activity");
    var statusPill = document.getElementById("dashboard-status-pill");

    loadDashboard();

    async function loadDashboard() {
        try {
            var results = await Promise.all([
                fetch("/runtime/status").then(safeJson),
                fetch("/model-providers/current").then(safeJson),
                fetch("/documents").then(safeJson),
                fetch("/collections").then(safeJson),
                fetch("/agent/tasks").then(safeJson)
            ]);

            var runtime = results[0];
            var current = results[1];
            var docs = results[2];
            var cols = results[3];
            var tasks = results[4];

            if (runtime) {
                var profile = runtime.activeProfile || "default";
                runtimeMode.textContent = profile === "default" ? "mock / 默认" : profile;
                statusPill.textContent = profile.indexOf("local") >= 0 ? "local-ai" : "mock";
                statusPill.classList.add("active");
            } else {
                runtimeMode.textContent = "暂未配置";
            }

            if (current && llm) {
                llm.textContent = formatModel(current.llmProviderName, current.llmModel);
            }

            if (Array.isArray(docs) && documents) {
                documents.textContent = docs.length + " 个文档";
                renderRecentDocuments(docs);
            }

            if (Array.isArray(cols) && collections) {
                collections.textContent = cols.length + " 个分组";
            }

            if (Array.isArray(tasks) && tasks.length > 0 && agentTask) {
                var latest = tasks[0];
                agentTask.textContent = (latest.title || "任务") + " · " + (latest.status || "-");
                renderRecentActivity(tasks);
            }
        } catch (e) {
            runtimeMode.textContent = "暂无数据";
        }
    }

    function renderRecentDocuments(docs) {
        if (!recentDocuments || !docs.length) {
            return;
        }
        var items = docs.slice(0, 5).map(function (doc) {
            var name = doc.originalFileName || doc.fileName || doc.title || "未命名文档";
            var status = doc.status || doc.lifecycleStatus || "";
            return '<li class="mw-activity-item"><strong>' + escapeHtml(name) + '</strong><span class="muted">' + escapeHtml(status) + '</span></li>';
        });
        recentDocuments.innerHTML = items.join("");
    }

    function renderRecentActivity(tasks) {
        if (!recentActivity || !tasks.length) {
            return;
        }
        var items = tasks.slice(0, 5).map(function (task) {
            var title = task.title || "AI 任务";
            var status = task.status || "-";
            return '<li class="mw-activity-item"><strong>' + escapeHtml(title) + '</strong><span class="muted">' + escapeHtml(status) + '</span></li>';
        });
        recentActivity.innerHTML = items.join("");
    }

    function escapeHtml(text) {
        return String(text)
            .replace(/&/g, "&amp;")
            .replace(/</g, "&lt;")
            .replace(/>/g, "&gt;")
            .replace(/"/g, "&quot;");
    }

    function formatModel(provider, model) {
        if (!provider && !model) {
            return "暂未配置";
        }
        return (provider || "-") + " / " + (model || "-");
    }

    function safeJson(response) {
        if (!response || !response.ok) {
            return null;
        }
        return response.json();
    }
})();
