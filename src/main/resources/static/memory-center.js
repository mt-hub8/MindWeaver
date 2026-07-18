(function () {
    var TYPES = ["PREFERENCE", "FACT", "DECISION", "SUMMARY", "CONSTRAINT", "FEEDBACK", "TASK_RESULT", "PROJECT_STATE", "AGENT_INSTRUCTION", "RISK_NOTE"];
    var SCOPES = ["USER", "PROJECT", "AGENT", "TASK", "SHARED"];
    var STATUSES = ["ACTIVE", "ARCHIVED", "DELETED", "EXPIRED", "CONFLICTED"];
    var SOURCES = ["MANUAL", "ASK_ANSWER", "AGENT_TASK", "EVALUATION_RUN", "DOCUMENT_SUMMARY", "SYSTEM_CREATED"];
    var LABELS = {
        PREFERENCE: "用户偏好", FACT: "事实", DECISION: "决策", SUMMARY: "摘要", CONSTRAINT: "约束",
        FEEDBACK: "反馈", TASK_RESULT: "任务结果", PROJECT_STATE: "项目状态",
        AGENT_INSTRUCTION: "Agent 指令", RISK_NOTE: "风险记录",
        USER: "用户级", PROJECT: "项目级", AGENT: "Agent 私有", TASK: "任务级", SHARED: "共享",
        MANUAL: "手动", ASK_ANSWER: "Ask 回答", AGENT_TASK: "Agent Task", EVALUATION_RUN: "评测运行",
        DOCUMENT_SUMMARY: "文档摘要", SYSTEM_CREATED: "系统创建",
        ACTIVE: "生效", ARCHIVED: "已归档", DELETED: "已删除", EXPIRED: "已过期", CONFLICTED: "有冲突"
    };
    var memories = [];
    var agents = [];
    var settings = { enabled: true };
    function $(id) { return document.getElementById(id); }
    function option(select, value, label) {
        var el = document.createElement("option"); el.value = value; el.textContent = label; select.appendChild(el);
    }
    function fillOptions(id, values) { values.forEach(function (v) { option($(id), v, LABELS[v] || v); }); }
    function fetchJson(url, options) {
        return fetch(url, options).then(function (res) {
            if (res.status === 204) return null;
            return res.json().then(function (body) {
                if (!res.ok) throw new Error(body.message || ("HTTP " + res.status));
                return body;
            });
        });
    }
    function initOptions() {
        fillOptions("filter-type", TYPES); fillOptions("filter-scope", SCOPES);
        fillOptions("filter-status", STATUSES); fillOptions("filter-source", SOURCES);
        fillOptions("memory-type", TYPES); fillOptions("memory-scope", SCOPES); fillOptions("memory-source", SOURCES);
    }
    function loadAgents() {
        return fetchJson("/agent-profiles").then(function (data) {
            agents = data || [];
            ["filter-agent", "memory-agent"].forEach(function (id) {
                var select = $(id);
                agents.forEach(function (agent) { option(select, agent.id, agent.displayName); });
            });
        });
    }
    function loadSettings() {
        return fetchJson("/memories/settings").then(function (data) {
            settings = data;
            $("memory-toggle").textContent = data.enabled ? "关闭记忆读取" : "启用记忆读取";
        });
    }
    function loadMemories() {
        var params = new URLSearchParams();
        [["keyword", "filter-keyword"], ["memoryType", "filter-type"], ["memoryScope", "filter-scope"],
            ["agentProfileId", "filter-agent"], ["status", "filter-status"], ["sourceType", "filter-source"]]
            .forEach(function (pair) { var v = $(pair[1]).value.trim(); if (v) params.set(pair[0], v); });
        return fetchJson("/memories?" + params.toString()).then(function (data) {
            memories = data || []; renderMemories(); renderStats();
        });
    }
    function renderStats() {
        var counts = { total: memories.length, USER: 0, PROJECT: 0, AGENT: 0, SHARED: 0, CONFLICTED: 0, lowConfidence: 0, expired: 0 };
        var now = Date.now();
        memories.forEach(function (m) {
            if (counts[m.memoryScope] != null) counts[m.memoryScope]++;
            if (m.status === "CONFLICTED") counts.CONFLICTED++;
            if (m.confidence < 0.5) counts.lowConfidence++;
            if (m.status === "EXPIRED" || (m.expiresAt && new Date(m.expiresAt).getTime() <= now)) counts.expired++;
        });
        Object.keys(counts).forEach(function (key) {
            var el = document.querySelector('[data-stat="' + key + '"]'); if (el) el.textContent = counts[key];
        });
    }
    function escapeHtml(value) {
        return String(value == null ? "" : value).replace(/[&<>"']/g, function (c) {
            return {"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c];
        });
    }
    function renderMemories() {
        var list = $("memory-list"); list.innerHTML = "";
        if (!memories.length) { list.innerHTML = '<div class="memory-empty">没有符合条件的记忆。</div>'; return; }
        memories.forEach(function (m) {
            var item = document.createElement("article"); item.className = "memory-item";
            var warning = m.confidence < 0.5 ? " warning" : "";
            var danger = m.status === "CONFLICTED" ? " danger" : "";
            item.innerHTML =
                '<div class="memory-item-head"><div><h3>' + escapeHtml(m.title) + '</h3>' +
                '<div class="memory-meta"><span class="memory-chip">' + (LABELS[m.memoryType] || m.memoryType) + '</span>' +
                '<span class="memory-chip">' + (LABELS[m.memoryScope] || m.memoryScope) + '</span>' +
                '<span class="memory-chip">' + (LABELS[m.sourceType] || m.sourceType) + '</span>' +
                '<span class="memory-chip' + warning + '">可信度 ' + m.confidence + '</span>' +
                '<span class="memory-chip">重要性 ' + m.importance + '</span>' +
                '<span class="memory-chip' + danger + '">' + (LABELS[m.status] || m.status) + '</span></div></div>' +
                '<span class="muted">' + escapeHtml((m.updatedAt || "").replace("T", " ").slice(0, 19)) + '</span></div>' +
                '<p class="memory-content">' + escapeHtml(m.content) + '</p>' +
                '<div class="memory-actions">' +
                '<button class="mw-button mw-button-secondary" data-action="edit">编辑</button>' +
                (m.status === "ACTIVE" ? '<button class="mw-button mw-button-secondary" data-action="archive">归档</button>' : '') +
                (m.status === "ARCHIVED" || m.status === "DELETED" ? '<button class="mw-button mw-button-secondary" data-action="restore">恢复</button>' : '') +
                (m.memoryScope === "AGENT" ? '<button class="mw-button mw-button-secondary" data-action="share">共享</button>' : '') +
                (m.memoryScope === "SHARED" && m.agentProfileId ? '<button class="mw-button mw-button-secondary" data-action="unshare">取消共享</button>' : '') +
                (m.status === "CONFLICTED" ? '<button class="mw-button mw-button-secondary" data-action="resolve">标记冲突已解决</button>' : '') +
                (m.status !== "DELETED" ? '<button class="mw-button mw-button-secondary" data-action="delete">删除</button>' : '') +
                '</div><details><summary>技术详情</summary><pre>' +
                escapeHtml(JSON.stringify({id:m.id, memoryKey:m.memoryKey, sourceId:m.sourceId, projectId:m.projectId,
                    agentProfileId:m.agentProfileId, taskId:m.taskId, metadataJson:m.metadataJson}, null, 2)) +
                '</pre></details>';
            item.addEventListener("click", function (event) {
                var action = event.target.getAttribute("data-action"); if (action) handleAction(action, m);
            });
            list.appendChild(item);
        });
    }
    function handleAction(action, memory) {
        if (action === "edit") return openDialog(memory);
        if (action === "delete" && !confirm("确认软删除这条记忆？")) return;
        var endpoint = action === "delete" ? "/memories/" + memory.id :
            action === "share" ? "/agent-profiles/" + memory.agentProfileId + "/memories/" + memory.id + "/share" :
            action === "unshare" ? "/agent-profiles/" + memory.agentProfileId + "/memories/" + memory.id + "/unshare" :
            "/memories/" + memory.id + "/" + (action === "resolve" ? "resolve-conflict" : action);
        var method = action === "delete" ? "DELETE" : "POST";
        fetchJson(endpoint, {method: method}).then(refreshAll).catch(showError);
    }
    function openDialog(memory) {
        $("memory-dialog-title").textContent = memory ? "编辑记忆" : "新增记忆";
        $("memory-id").value = memory ? memory.id : "";
        $("memory-title").value = memory ? memory.title : "";
        $("memory-content").value = memory ? memory.content : "";
        $("memory-type").value = memory ? memory.memoryType : "FACT";
        $("memory-scope").value = memory ? memory.memoryScope : "USER";
        $("memory-source").value = memory ? memory.sourceType : "MANUAL";
        $("memory-visibility").value = memory ? memory.visibility : "PRIVATE";
        $("memory-agent").value = memory && memory.agentProfileId ? memory.agentProfileId : "";
        $("memory-project").value = memory && memory.projectId ? memory.projectId : "";
        $("memory-task").value = memory && memory.taskId ? memory.taskId : "";
        $("memory-key").value = memory && memory.memoryKey ? memory.memoryKey : "";
        $("memory-confidence").value = memory ? memory.confidence : 1;
        $("memory-importance").value = memory ? memory.importance : 50;
        $("memory-expires").value = memory && memory.expiresAt ? memory.expiresAt.slice(0, 16) : "";
        $("memory-metadata").value = memory && memory.metadataJson ? memory.metadataJson : "";
        $("memory-form-error").textContent = "";
        $("memory-dialog").showModal();
    }
    function formPayload() {
        function numberOrNull(id) { return $(id).value ? Number($(id).value) : null; }
        return {
            memoryKey: $("memory-key").value.trim() || null, title: $("memory-title").value.trim(),
            content: $("memory-content").value.trim(), memoryType: $("memory-type").value,
            memoryScope: $("memory-scope").value, visibility: $("memory-visibility").value,
            sourceType: $("memory-source").value, sourceId: null, projectId: numberOrNull("memory-project"),
            agentProfileId: numberOrNull("memory-agent"), taskId: numberOrNull("memory-task"),
            confidence: Number($("memory-confidence").value), importance: Number($("memory-importance").value),
            expiresAt: $("memory-expires").value || null, metadataJson: $("memory-metadata").value.trim() || null
        };
    }
    function saveMemory(event) {
        event.preventDefault();
        var id = $("memory-id").value;
        fetchJson(id ? "/memories/" + id : "/memories", {
            method: id ? "PUT" : "POST", headers: {"Content-Type":"application/json"}, body: JSON.stringify(formPayload())
        }).then(function () { $("memory-dialog").close(); return refreshAll(); })
            .catch(function (error) { $("memory-form-error").textContent = error.message; });
    }
    function loadDiagnostics() {
        return fetchJson("/memories/diagnostics").then(function (report) {
            var list = $("diagnostics-list"); list.innerHTML = "";
            if (!report.issues.length) list.innerHTML = '<div class="memory-empty">当前未发现记忆诊断问题。</div>';
            report.issues.forEach(function (issue) {
                var el = document.createElement("div"); el.className = "diagnostic-item";
                el.innerHTML = "<strong>" + escapeHtml(issue.title) + "</strong><p>" + escapeHtml(issue.description) +
                    "</p><span class='muted'>涉及记忆：" + escapeHtml((issue.memoryIds || []).join(", ")) + "</span>";
                list.appendChild(el);
            });
            $("diagnostics-suggestions").innerHTML = (report.suggestions || []).map(function (s) { return "<li>" + escapeHtml(s) + "</li>"; }).join("");
        });
    }
    function refreshAll() { return Promise.all([loadMemories(), loadDiagnostics(), loadSettings()]); }
    function showError(error) { alert(error.message || String(error)); }
    initOptions();
    loadAgents().then(function () {
        var query = new URLSearchParams(location.search);
        if (query.get("agentProfileId")) {
            $("filter-agent").value = query.get("agentProfileId");
        }
        return refreshAll().then(function () {
            if (query.get("create") === "true") {
                openDialog(null);
                $("memory-scope").value = "AGENT";
                $("memory-agent").value = query.get("agentProfileId") || "";
                $("memory-type").value = "AGENT_INSTRUCTION";
            }
        });
    }).catch(showError);
    $("new-memory").addEventListener("click", function () { openDialog(null); });
    $("close-memory-dialog").addEventListener("click", function () { $("memory-dialog").close(); });
    $("memory-form").addEventListener("submit", saveMemory);
    $("refresh-memory").addEventListener("click", loadMemories);
    $("refresh-diagnostics").addEventListener("click", loadDiagnostics);
    $("memory-toggle").addEventListener("click", function () {
        fetchJson("/memories/settings/enabled?enabled=" + (!settings.enabled), {method:"POST"}).then(loadSettings).catch(showError);
    });
    ["filter-type", "filter-scope", "filter-agent", "filter-status", "filter-source"].forEach(function (id) {
        $(id).addEventListener("change", loadMemories);
    });
    var timer; $("filter-keyword").addEventListener("input", function () { clearTimeout(timer); timer = setTimeout(loadMemories, 250); });
})();
