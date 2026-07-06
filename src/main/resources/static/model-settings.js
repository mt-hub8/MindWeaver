(function () {
    const loadingStatus = document.getElementById("loading-status");
    const errorStatus = document.getElementById("error-status");
    const errorMessage = document.getElementById("error-message");
    const currentPanel = document.getElementById("current-panel");
    const providersPanel = document.getElementById("providers-panel");
    const formPanel = document.getElementById("form-panel");
    const currentModelBody = document.getElementById("current-model-body");
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

    let presets = [];
    let providers = [];
    let editingId = null;

    document.getElementById("add-provider-button").addEventListener("click", function () {
        openForm(null);
    });
    document.getElementById("cancel-form-button").addEventListener("click", hideForm);
    document.getElementById("save-provider-button").addEventListener("click", saveProvider);
    document.getElementById("scroll-to-providers-btn").addEventListener("click", function () {
        providersPanel.scrollIntoView({ behavior: "smooth" });
    });
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
        runtimeMode.textContent = current.runtimeModeDescription || "";
        if (current.embeddingDimensionMismatchWarning) {
            dimensionWarning.textContent = current.embeddingDimensionMismatchMessage
                || "切换 Embedding 模型后，已有文档可能需要重新索引。";
            dimensionWarning.classList.remove("hidden");
        } else {
            dimensionWarning.classList.add("hidden");
        }

        var llmName = (current.llmProviderName || "未配置") + " / " + (current.llmModel || "-");
        var embName = (current.embeddingProviderName || "未配置") + " / " + (current.embeddingModel || "-");
        var tags = '<div class="capability-tags">' +
            '<span class="capability-tag">LLM</span>' +
            '<span class="capability-tag">Embedding</span>' +
            '<span class="capability-tag">' + escapeHtml(current.activeProfile || "mock") + '</span></div>';

        currentModelBody.innerHTML =
            '<div><h3 class="provider-name" style="font-size:1.1rem">' + escapeHtml(current.llmModel || "当前模型") + '</h3>' +
            '<p class="provider-type-label">' + escapeHtml(current.llmProviderName || "默认 Provider") + '</p>' +
            tags + '</div>' +
            '<div><div class="provider-field-label">默认 LLM</div><div class="provider-field-value">' + escapeHtml(llmName) + '</div></div>' +
            '<div><div class="provider-field-label">默认 Embedding</div><div class="provider-field-value">' + escapeHtml(embName) + '</div></div>' +
            '<div></div>';
    }

    function providerInitials(name) {
        if (!name) return "?";
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
            var card = document.createElement("div");
            var selected = p.defaultLlm || p.defaultEmbedding;
            card.className = "provider-card" + (selected ? " selected" : "");
            card.innerHTML =
                '<div class="provider-card-inner">' +
                '<div class="provider-identity">' +
                '<div class="provider-avatar">' + escapeHtml(providerInitials(p.displayName)) + '</div>' +
                '<div><h3 class="provider-name">' + escapeHtml(p.displayName) + '</h3>' +
                '<p class="provider-type-label">' + escapeHtml(formatProviderType(p.providerType)) + '</p>' +
                '<p class="provider-desc">' + escapeHtml(providerDescription(p)) + '</p></div></div>' +
                '<div><div class="provider-status-row"><span class="status-dot ' + connectionDotClass(p) + '"></span>' +
                '<span>' + escapeHtml(connectionStatusText(p)) + '</span></div>' +
                '<div class="provider-field-value" style="font-size:0.82rem;color:var(--text-muted)">' + escapeHtml(apiKeyStatusText(p)) + '</div></div>' +
                '<div><div class="provider-field-label">默认 LLM</div><div class="provider-field-value">' + escapeHtml(p.defaultLlmModel || "-") + (p.defaultLlm ? " ★" : "") + '</div></div>' +
                '<div><div class="provider-field-label">默认 Embedding</div><div class="provider-field-value">' + escapeHtml(p.defaultEmbeddingModel || "-") + (p.defaultEmbedding ? " ★" : "") + '</div></div>' +
                '<div class="provider-actions"></div></div>' +
                '<details class="tech-details"><summary>查看技术详情</summary><pre>' +
                escapeHtml(JSON.stringify({ id: p.id, providerType: p.providerType, enabled: p.enabled }, null, 2)) + '</pre></details>';
            var actions = card.querySelector(".provider-actions");
            actions.appendChild(actionButton("测试连接", function () { testProvider(p.id); }));
            actions.appendChild(moreMenuButton(p));
            providersList.appendChild(card);
        });
    }

    function moreMenuButton(p) {
        var wrap = document.createElement("div");
        wrap.style.display = "flex";
        wrap.style.flexDirection = "column";
        wrap.style.gap = "4px";
        wrap.style.alignItems = "flex-end";
        wrap.appendChild(actionButton("设为默认问答模型", function () { setDefault(p.id, "llm"); }));
        wrap.appendChild(actionButton("设为默认向量模型", function () { setDefault(p.id, "embedding"); }));
        wrap.appendChild(actionButton("编辑", function () { openForm(p); }));
        return wrap;
    }

    function formatProviderType(type) {
        if (type === "OLLAMA") return "本地 Ollama";
        if (type === "MOCK") return "Mock（开发测试）";
        return "OpenAI-compatible";
    }

    function providerDescription(p) {
        if (p.providerType === "OLLAMA") return "本地模型运行，无需 API Key";
        if (p.providerType === "MOCK") return "开发测试用模拟模型";
        return "通过 API 密钥连接远程服务";
    }

    function connectionDotClass(p) {
        if (!p.enabled) return "warning";
        if (p.lastTestStatus === "SUCCESS") return "success";
        if (p.lastTestStatus === "FAILED") return "error";
        return "warning";
    }

    function connectionStatusText(p) {
        if (!p.enabled) return "已禁用";
        if (!p.lastTestStatus) return "未测试";
        if (p.lastTestStatus === "SUCCESS") return "已连接";
        return "测试失败";
    }

    function apiKeyStatusText(p) {
        if (p.providerType === "OLLAMA" || p.providerType === "MOCK") {
            return "本地模型无需密钥";
        }
        if (p.apiKeyMasked) return "API Key 已配置";
        return "API Key 未配置";
    }

    function actionButton(label, handler) {
        const btn = document.createElement("button");
        btn.type = "button";
        btn.className = "link-button";
        btn.textContent = label;
        btn.addEventListener("click", handler);
        return btn;
    }

    function formatEnabled(p) {
        return p.enabled ? "已启用" : "已禁用";
    }

    function formatTest(p) {
        if (!p.lastTestStatus) {
            return "未测试";
        }
        if (p.lastTestStatus === "SUCCESS") {
            return "连接正常";
        }
        return "连接异常";
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
