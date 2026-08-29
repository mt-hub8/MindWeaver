(function () {
    var profiles = [];
    function $(id) { return document.getElementById(id); }
    function escapeHtml(value) {
        return String(value == null ? "" : value).replace(/[&<>"']/g, function (c) {
            return {"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c];
        });
    }
    function fetchJson(url, options) {
        return fetch(url, options).then(function (res) {
            if (res.status === 204) return null;
            return res.json().then(function (body) {
                if (!res.ok) throw new Error(body.message || ("HTTP " + res.status));
                return body;
            });
        });
    }
    function load() {
        return fetchJson("/agent-profiles").then(function (data) { profiles = data || []; render(); });
    }
    function render() {
        var grid = $("profile-grid"); grid.innerHTML = "";
        profiles.forEach(function (p) {
            var card = document.createElement("article"); card.className = "profile-card";
            card.innerHTML =
                '<div class="profile-card-head"><div><h3>' + escapeHtml(p.displayName) + '</h3><span class="muted">' +
                escapeHtml(p.agentKey) + " · " + escapeHtml(p.roleName) + '</span></div>' +
                '<span class="memory-chip ' + (p.enabled ? "" : "warning") + '">' + (p.enabled ? "已启用" : "已禁用") + '</span></div>' +
                '<p class="memory-content">' + escapeHtml(p.description || "未填写职责") + '</p>' +
                '<div class="profile-instruction">' + escapeHtml(p.systemInstruction) + '</div>' +
                '<div class="memory-meta"><span class="memory-chip">私有记忆 ' + p.privateMemoryCount + '</span>' +
                '<span class="memory-chip">共享记忆 ' + p.sharedMemoryCount + '</span>' +
                '<span class="memory-chip">默认作用域 ' + p.defaultMemoryScope + '</span></div>' +
                '<div class="memory-actions"><button class="mw-button mw-button-secondary" data-action="edit">编辑角色</button>' +
                '<button class="mw-button mw-button-secondary" data-action="toggle">' + (p.enabled ? "禁用" : "启用") + '</button>' +
                '<button class="mw-button mw-button-secondary" data-action="view-memory">查看角色记忆</button>' +
                '<button class="mw-button mw-button-secondary" data-action="add-memory">添加角色记忆</button>' +
                '<button class="mw-button mw-button-secondary" data-action="delete">删除角色</button></div>';
            card.addEventListener("click", function (event) {
                var action = event.target.getAttribute("data-action"); if (action) act(action, p);
            });
            grid.appendChild(card);
        });
        if (!profiles.length) grid.innerHTML = '<div class="memory-empty">还没有角色。</div>';
    }
    function act(action, p) {
        if (action === "edit") return openDialog(p);
        if (action === "view-memory") return location.href = "/memory-center.html?agentProfileId=" + p.id;
        if (action === "add-memory") return location.href = "/memory-center.html?agentProfileId=" + p.id + "&create=true";
        if (action === "delete") {
            if (!confirm("删除角色不会删除其记忆，这些记忆将由诊断标记为孤儿。确认删除？")) return;
            return fetchJson("/agent-profiles/" + p.id, {method:"DELETE"}).then(load).catch(showError);
        }
        fetchJson("/agent-profiles/" + p.id + "/" + (p.enabled ? "disable" : "enable"), {method:"POST"})
            .then(load).catch(showError);
    }
    function openDialog(p) {
        $("profile-dialog-title").textContent = p ? "编辑角色" : "新建角色";
        $("profile-id").value = p ? p.id : "";
        $("profile-key").value = p ? p.agentKey : "";
        $("profile-name").value = p ? p.displayName : "";
        $("profile-role").value = p ? p.roleName : "";
        $("profile-scope").value = p ? p.defaultMemoryScope : "AGENT";
        $("profile-description").value = p ? (p.description || "") : "";
        $("profile-instruction").value = p ? p.systemInstruction : "";
        $("profile-metadata").value = p ? (p.metadataJson || "") : "";
        $("profile-error").textContent = "";
        $("profile-dialog").showModal();
    }
    function save(event) {
        event.preventDefault();
        var id = $("profile-id").value;
        var existing = profiles.find(function (profile) { return String(profile.id) === String(id); });
        var payload = {
            agentKey:$("profile-key").value.trim(), displayName:$("profile-name").value.trim(),
            roleName:$("profile-role").value.trim(), description:$("profile-description").value.trim() || null,
            systemInstruction:$("profile-instruction").value.trim(), defaultMemoryScope:$("profile-scope").value,
            enabled:existing ? existing.enabled : true, metadataJson:$("profile-metadata").value.trim() || null
        };
        fetchJson(id ? "/agent-profiles/" + id : "/agent-profiles", {
            method:id ? "PUT" : "POST", headers:{"Content-Type":"application/json"}, body:JSON.stringify(payload)
        }).then(function () { $("profile-dialog").close(); return load(); })
            .catch(function (error) { $("profile-error").textContent = error.message; });
    }
    function showError(error) { alert(error.message || String(error)); }
    $("new-profile").addEventListener("click", function () { openDialog(null); });
    $("close-profile-dialog").addEventListener("click", function () { $("profile-dialog").close(); });
    $("profile-form").addEventListener("submit", save);
    load().catch(showError);
})();
