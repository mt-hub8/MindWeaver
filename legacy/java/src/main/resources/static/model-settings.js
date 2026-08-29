(function () {
    const loadingStatus = document.getElementById("loading-status");
    const errorStatus = document.getElementById("error-status");
    const errorMessage = document.getElementById("error-message");
    const currentPanel = document.getElementById("current-panel");
    const providersPanel = document.getElementById("providers-panel");
    const formPanel = document.getElementById("form-panel");
    const runtimeMode = document.getElementById("runtime-mode");
    const dimensionWarning = document.getElementById("dimension-warning");
    const providersList = document.getElementById("providers-list");
    const providersEmpty = document.getElementById("providers-empty");
    const presetSelect = document.getElementById("preset-select");
    const formTitle = document.getElementById("form-title");
    const formResult = document.getElementById("form-result");
    const formTechDetails = document.getElementById("form-tech-details");
    const formTechJson = document.getElementById("form-tech-json");
    const actionNotice = document.getElementById("action-notice");
    const actionNoticeMessage = document.getElementById("action-notice-message");
    const currentTechBody = document.getElementById("current-tech-body");

    let presets = [];
    let providers = [];
    let editingId = null;
    let currentStatus = null;

    document.getElementById("add-provider-button").addEventListener("click", function () {
        openForm(null);
    });
    document.getElementById("cancel-form-button").addEventListener("click", hideForm);
    document.getElementById("save-provider-button").addEventListener("click", saveProvider);
    document.getElementById("scroll-to-providers-btn").addEventListener("click", function () {
        providersPanel.scrollIntoView({ behavior: "smooth" });
    });
    document.getElementById("test-current-connection-btn").addEventListener("click", testCurrentConnection);
    presetSelect.addEventListener("change", applyPreset);

    loadAll();

    async function loadAll() {
        setLoading(true);
        clearError();
        try {
            const [currentRes, listRes, presetsRes] = await Promise.all([
                fetch("/model-providers/current"),
                fetch("/model-providers"),
                fetch("/model-providers/presets")
            ]);
            if (!currentRes.ok || !listRes.ok || !presetsRes.ok) {
                showError("加载模型设置失败");
                return;
            }
            const current = await currentRes.json();
            providers = await listRes.json();
            presets = await presetsRes.json();
            renderPresets();
            renderCurrent(current);
            renderProviders();
            currentPanel.classList.remove("hidden");
            providersPanel.classList.remove("hidden");
        } catch (e) {
            showError("无法连接服务");
        } finally {
            setLoading(false);
        }
    }

    function renderPresets() {
        presetSelect.innerHTML = '<option value="">手动填写</option>';
        presets.forEach(function (p) {
            const opt = document.createElement("option");
            opt.value = p.presetId;
            opt.textContent = p.displayName;
            presetSelect.appendChild(opt);
        });
    }

    function renderCurrent(current) {
        currentStatus = current;
        runtimeMode.textContent = current.runtimeModeDescription || "";

        if (current.embeddingDimensionMismatchWarning) {
            dimensionWarning.textContent = current.embeddingDimensionMismatchMessage
                || "切换 Embedding 模型后，已有文档可能需要重新索引。";
            dimensionWarning.classList.remove("hidden");
        } else {
            dimensionWarning.classList.add("hidden");
        }

        setText("current-llm-model", current.llmModel || "未配置");
        setText("current-embedding-model", current.embeddingModel || "未配置");
        setText("current-llm-hint", llmUsageHint(current));
        setText("current-embedding-hint", "用于文档向量化与语义检索。");
        setText("current-llm-provider", providerLine(current.llmProviderName, "问答模型供应商"));
        setText("current-embedding-provider", providerLine(current.embeddingProviderName, "向量模型供应商"));
        setText("current-runtime-label", runtimeChainLabel(current));
        setText("current-runtime-hint", runtimeChainHint(current));
        setText("current-provider-status", providerStatusSummary(current));

        if (currentTechBody) {
            currentTechBody.innerHTML = buildCurrentTechDetails(current);
        }
    }

    function setText(id, value) {
        var el = document.getElementById(id);
        if (el) {
            el.textContent = value || "";
        }
    }

    function llmUsageHint(current) {
        if (!current.llmModel) {
            return "尚未配置默认问答模型。";
        }
        return "用于生成回答、摘要和任务报告。";
    }

    function providerLine(name, fallback) {
        if (!name) {
            return fallback + "：未配置";
        }
        return fallback + "：" + name;
    }

    function runtimeChainLabel(current) {
        var desc = (current.runtimeModeDescription || "").toLowerCase();
        if (desc.indexOf("ollama") >= 0 || desc.indexOf("python worker") >= 0) {
            return "本地模型服务";
        }
        if (current.activeProfile && current.activeProfile.indexOf("local") >= 0) {
            return "本地模型服务";
        }
        if (current.activeProfile === "default" || desc.indexOf("mock") >= 0) {
            return "开发 / Mock 模式";
        }
        if (current.llmModel || current.embeddingModel) {
            return "已配置模型链路";
        }
        return "未配置";
    }

    function runtimeChainHint(current) {
        var desc = current.runtimeModeDescription || "";
        if (desc.indexOf("Ollama") >= 0 && desc.indexOf("Python Worker") >= 0) {
            return "通过 Ollama 与本地 Worker 调用，不依赖云端。";
        }
        if (desc.indexOf("Ollama") >= 0) {
            return "通过本地 Ollama 服务调用。";
        }
        if (desc.indexOf("Python Worker") >= 0) {
            return "通过本地 Python Worker 调用。";
        }
        if (desc.indexOf("application.properties") >= 0) {
            return "配置来源见下方技术详情。";
        }
        if (current.llmProviderName) {
            return "由 " + current.llmProviderName + " 提供模型能力。";
        }
        return "完成供应商配置后可在此查看运行链路。";
    }

    function providerStatusSummary(current) {
        var profile = current.activeProfile || "default";
        if (profile === "default") {
            return "运行配置：mock / 默认";
        }
        if (profile.indexOf("local") >= 0) {
            return "运行配置：local-ai · 已启用";
        }
        return "运行配置：" + profile;
    }

    function buildCurrentTechDetails(current) {
        var rows = [
            ["运行模式说明", current.runtimeModeDescription || "—"],
            ["Active Profile", current.activeProfile || "—"],
            ["LLM Provider", formatProviderRef(current.llmProviderName, current.llmProviderConfigId)],
            ["Embedding Provider", formatProviderRef(current.embeddingProviderName, current.embeddingProviderConfigId)],
            ["Embedding 维度", current.embeddingDimension != null ? String(current.embeddingDimension) : "—"]
        ];
        return "<dl>" + rows.map(function (row) {
            return "<dt>" + escapeHtml(row[0]) + "</dt><dd>" + escapeHtml(row[1]) + "</dd>";
        }).join("") + "</dl>";
    }

    function formatProviderRef(name, id) {
        if (!name && (id == null || id === "")) {
            return "未配置";
        }
        var label = name || "未命名";
        if (id != null && id !== "") {
            return label + " (ID: " + id + ")";
        }
        return label;
    }

    async function testCurrentConnection() {
        if (!currentStatus) {
            showError("当前模型信息尚未加载");
            return;
        }
        var targetId = currentStatus.llmProviderConfigId || currentStatus.embeddingProviderConfigId;
        if (!targetId) {
            showError("当前没有可测试的模型供应商");
            return;
        }
        await testProvider(targetId);
    }

    function providerInitials(name) {
        if (!name) {
            return "?";
        }
        return name.substring(0, 2).toUpperCase();
    }

    function renderProviders() {
        providersList.innerHTML = "";
        if (!providers.length) {
            providersEmpty.classList.remove("hidden");
            return;
        }
        providersEmpty.classList.add("hidden");
        providers.forEach(function (p) {
            var card = document.createElement("article");
            var selected = p.defaultLlm || p.defaultEmbedding;
            card.className = "mw-card provider-card" + (selected ? " selected" : "");
            card.innerHTML =
                '<div class="provider-identity">' +
                '<div class="provider-logo">' + escapeHtml(providerInitials(p.displayName)) + "</div>" +
                "<div>" +
                '<h3 class="provider-name">' + escapeHtml(p.displayName) + (p.defaultLlm ? ' <span class="mw-badge mw-badge-accent">默认 LLM</span>' : "") +
                (p.defaultEmbedding ? ' <span class="mw-badge mw-badge-accent">默认 Embedding</span>' : "") + "</h3>" +
                '<p class="provider-type-label">' + escapeHtml(formatProviderType(p.providerType)) + "</p>" +
                '<p class="provider-desc">' + escapeHtml(providerDescription(p)) + "</p>" +
                "</div></div>" +
                '<div class="provider-meta"><span class="provider-meta-label">默认 LLM</span>' +
                '<strong class="provider-meta-value">' + escapeHtml(p.defaultLlmModel || "—") + "</strong></div>" +
                '<div class="provider-meta"><span class="provider-meta-label">默认 Embedding</span>' +
                '<strong class="provider-meta-value">' + escapeHtml(p.defaultEmbeddingModel || "—") + "</strong></div>" +
                '<div class="provider-status">' +
                '<span class="provider-status-main"><span class="status-dot ' + connectionDotClass(p) + '"></span>' +
                escapeHtml(connectionStatusText(p)) + "</span>" +
                '<span class="provider-status-sub">' + escapeHtml(apiKeyStatusText(p)) + "</span></div>" +
                '<div class="provider-actions"></div>' +
                '<details class="provider-card-tech technical-details"><summary>查看技术详情</summary><pre>' +
                escapeHtml(JSON.stringify({
                    id: p.id,
                    providerType: p.providerType,
                    enabled: p.enabled,
                    baseUrl: p.baseUrl || null,
                    embeddingDimension: p.embeddingDimension,
                    lastTestStatus: p.lastTestStatus || null
                }, null, 2)) + "</pre></details>";

            var actions = card.querySelector(".provider-actions");
            actions.appendChild(actionButton("测试连接", function () { testProvider(p.id); }, "mw-button-secondary"));
            actions.appendChild(actionButton("编辑", function () { openForm(p); }, "mw-button-ghost"));
            actions.appendChild(moreMenuButton(p));
            providersList.appendChild(card);
        });
    }

    function moreMenuButton(p) {
        var details = document.createElement("details");
        details.className = "provider-more-menu";
        details.innerHTML = '<summary class="mw-button mw-button-ghost">更多</summary><div class="provider-more-menu-body"></div>';
        var body = details.querySelector(".provider-more-menu-body");
        body.appendChild(actionButton("设为默认问答模型", function () { setDefault(p.id, "llm"); }, "mw-button-ghost"));
        body.appendChild(actionButton("设为默认向量模型", function () { setDefault(p.id, "embedding"); }, "mw-button-ghost"));
        if (p.enabled) {
            body.appendChild(actionButton("停用供应商", function () { toggleEnable(p.id, false); }, "mw-button-ghost"));
        } else {
            body.appendChild(actionButton("启用供应商", function () { toggleEnable(p.id, true); }, "mw-button-ghost"));
        }
        return details;
    }

    function formatProviderType(type) {
        if (type === "OLLAMA") {
            return "本地 Ollama";
        }
        if (type === "MOCK") {
            return "Mock（开发测试）";
        }
        return "OpenAI-compatible";
    }

    function providerDescription(p) {
        if (p.providerType === "OLLAMA") {
            return "本地模型运行，无需 API Key";
        }
        if (p.providerType === "MOCK") {
            return "开发测试用模拟模型";
        }
        return "通过 API 密钥连接远程服务";
    }

    function connectionDotClass(p) {
        if (!p.enabled) {
            return "warning";
        }
        if (p.lastTestStatus === "SUCCESS") {
            return "success";
        }
        if (p.lastTestStatus === "FAILED") {
            return "danger";
        }
        return "warning";
    }

    function connectionStatusText(p) {
        if (!p.enabled) {
            return "已禁用";
        }
        if (!p.lastTestStatus) {
            return "未测试";
        }
        if (p.lastTestStatus === "SUCCESS") {
            return "已连接";
        }
        return "测试失败";
    }

    function apiKeyStatusText(p) {
        if (p.providerType === "OLLAMA" || p.providerType === "MOCK") {
            return "本地模型无需密钥";
        }
        if (p.apiKeyMasked) {
            return "API Key 已配置";
        }
        return "API Key 未配置";
    }

    function actionButton(label, handler, variant) {
        const btn = document.createElement("button");
        btn.type = "button";
        btn.className = "mw-button " + (variant || "mw-button-ghost");
        btn.textContent = label;
        btn.addEventListener("click", handler);
        return btn;
    }

    function openForm(provider) {
        editingId = provider ? provider.id : null;
        formTitle.textContent = provider ? "编辑模型供应商" : "添加模型供应商";
        formPanel.classList.remove("hidden");
        formResult.classList.add("hidden");
        document.getElementById("provider-type").value = provider ? provider.providerType : "OLLAMA";
        document.getElementById("display-name").value = provider ? provider.displayName : "";
        document.getElementById("base-url").value = provider ? (provider.baseUrl || "") : "";
        document.getElementById("api-key").value = "";
        document.getElementById("default-llm-model").value = provider ? (provider.defaultLlmModel || "") : "";
        document.getElementById("default-embedding-model").value = provider ? (provider.defaultEmbeddingModel || "") : "";
        document.getElementById("embedding-dimension").value = provider && provider.embeddingDimension != null
            ? provider.embeddingDimension : "";
        document.getElementById("enabled").checked = provider ? provider.enabled : true;
        if (provider) {
            formTechDetails.classList.remove("hidden");
            formTechJson.textContent = JSON.stringify(provider, null, 2);
        } else {
            formTechDetails.classList.add("hidden");
        }
        formPanel.scrollIntoView({ behavior: "smooth" });
    }

    function hideForm() {
        formPanel.classList.add("hidden");
        editingId = null;
    }

    function applyPreset() {
        const id = presetSelect.value;
        if (!id) {
            return;
        }
        const preset = presets.find(function (p) { return p.presetId === id; });
        if (!preset) {
            return;
        }
        document.getElementById("provider-type").value = preset.providerType;
        document.getElementById("display-name").value = preset.displayName || "";
        document.getElementById("base-url").value = preset.baseUrl || "";
        document.getElementById("default-llm-model").value = preset.defaultLlmModel || "";
        document.getElementById("default-embedding-model").value = preset.defaultEmbeddingModel || "";
        document.getElementById("embedding-dimension").value = preset.embeddingDimension != null
            ? preset.embeddingDimension : "";
    }

    async function saveProvider() {
        const body = {
            providerType: document.getElementById("provider-type").value,
            displayName: document.getElementById("display-name").value.trim(),
            baseUrl: document.getElementById("base-url").value.trim(),
            apiKey: document.getElementById("api-key").value,
            defaultLlmModel: document.getElementById("default-llm-model").value.trim(),
            defaultEmbeddingModel: document.getElementById("default-embedding-model").value.trim(),
            embeddingDimension: parseOptionalInt(document.getElementById("embedding-dimension").value),
            enabled: document.getElementById("enabled").checked
        };
        const url = editingId ? "/model-providers/" + editingId : "/model-providers";
        const method = editingId ? "PUT" : "POST";
        try {
            const response = await fetch(url, {
                method: method,
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify(body)
            });
            const payload = await response.json();
            if (!response.ok) {
                formResult.classList.remove("hidden");
                formResult.textContent = payload.message || "保存失败";
                return;
            }
            hideForm();
            await loadAll();
        } catch (e) {
            formResult.classList.remove("hidden");
            formResult.textContent = "保存失败";
        }
    }

    async function testProvider(id) {
        clearActionNotice();
        try {
            const response = await fetch("/model-providers/" + id + "/test", { method: "POST" });
            const payload = await response.json();
            if (payload.status === "SUCCESS") {
                showActionNotice(payload.message || "连接正常", true);
            } else {
                showError(payload.message || "连接异常");
            }
            await loadAll();
        } catch (e) {
            showError("测试连接失败");
        }
    }

    async function setDefault(id, kind) {
        const path = kind === "llm" ? "set-default-llm" : "set-default-embedding";
        clearActionNotice();
        try {
            const response = await fetch("/model-providers/" + id + "/" + path, { method: "POST" });
            const payload = await response.json();
            if (!response.ok) {
                showError(payload.message || "设置失败");
                return;
            }
            showActionNotice("默认模型已更新", true);
            await loadAll();
        } catch (e) {
            showError("设置失败");
        }
    }

    async function toggleEnable(id, enable) {
        const path = enable ? "enable" : "disable";
        clearActionNotice();
        try {
            const response = await fetch("/model-providers/" + id + "/" + path, { method: "POST" });
            const payload = await response.json();
            if (!response.ok) {
                showError(payload.message || "操作失败");
                return;
            }
            showActionNotice(enable ? "供应商已启用" : "供应商已停用", true);
            await loadAll();
        } catch (e) {
            showError("操作失败");
        }
    }

    function parseOptionalInt(value) {
        if (value === "" || value == null) {
            return null;
        }
        const n = Number.parseInt(value, 10);
        return Number.isNaN(n) ? null : n;
    }

    function setLoading(isLoading) {
        loadingStatus.classList.toggle("hidden", !isLoading);
        loadingStatus.classList.toggle("visible", isLoading);
    }

    function showError(msg) {
        clearActionNotice();
        errorMessage.textContent = msg;
        errorStatus.classList.remove("hidden");
    }

    function clearError() {
        errorStatus.classList.add("hidden");
        errorMessage.textContent = "";
    }

    function showActionNotice(msg, isSuccess) {
        clearError();
        if (!actionNotice || !actionNoticeMessage) {
            return;
        }
        actionNoticeMessage.textContent = msg;
        actionNotice.classList.remove("hidden");
        if (!isSuccess) {
            actionNotice.style.background = "var(--danger-soft)";
            actionNotice.style.borderColor = "var(--danger-border)";
        } else {
            actionNotice.style.background = "";
            actionNotice.style.borderColor = "";
        }
        window.setTimeout(clearActionNotice, 5000);
    }

    function clearActionNotice() {
        if (actionNotice) {
            actionNotice.classList.add("hidden");
        }
        if (actionNoticeMessage) {
            actionNoticeMessage.textContent = "";
        }
    }

    function escapeHtml(value) {
        if (value == null) {
            return "";
        }
        return String(value)
            .replace(/&/g, "&amp;")
            .replace(/</g, "&lt;")
            .replace(/>/g, "&gt;");
    }
})();
