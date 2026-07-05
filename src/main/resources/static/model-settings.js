(function () {
    const loadingStatus = document.getElementById("loading-status");
    const errorStatus = document.getElementById("error-status");
    const errorMessage = document.getElementById("error-message");
    const currentPanel = document.getElementById("current-panel");
    const providersPanel = document.getElementById("providers-panel");
    const formPanel = document.getElementById("form-panel");
    const currentSummary = document.getElementById("current-summary");
    const runtimeMode = document.getElementById("runtime-mode");
    const dimensionWarning = document.getElementById("dimension-warning");
    const providersBody = document.getElementById("providers-body");
    const providersEmpty = document.getElementById("providers-empty");
    const providersTable = document.getElementById("providers-table");
    const presetSelect = document.getElementById("preset-select");
    const formTitle = document.getElementById("form-title");
    const formResult = document.getElementById("form-result");
    const formTechDetails = document.getElementById("form-tech-details");
    const formTechJson = document.getElementById("form-tech-json");

    let presets = [];
    let providers = [];
    let editingId = null;

    document.getElementById("add-provider-button").addEventListener("click", function () {
        openForm(null);
    });
    document.getElementById("cancel-form-button").addEventListener("click", hideForm);
    document.getElementById("save-provider-button").addEventListener("click", saveProvider);
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
        currentSummary.innerHTML = "";
        addSummaryRow("当前运行模式", current.activeProfile || "-");
        addSummaryRow("当前 LLM Provider", (current.llmProviderName || "-") + " / " + (current.llmModel || "-"));
        addSummaryRow("当前 Embedding Provider", (current.embeddingProviderName || "-") + " / " + (current.embeddingModel || "-"));
        addSummaryRow("Embedding 维度", current.embeddingDimension != null ? String(current.embeddingDimension) : "-");
    }

    function addSummaryRow(label, value) {
        const dt = document.createElement("dt");
        dt.textContent = label;
        const dd = document.createElement("dd");
        dd.textContent = value;
        currentSummary.appendChild(dt);
        currentSummary.appendChild(dd);
    }

    function renderProviders() {
        providersBody.innerHTML = "";
        if (!providers.length) {
            providersEmpty.classList.remove("hidden");
            providersTable.classList.add("hidden");
            return;
        }
        providersEmpty.classList.add("hidden");
        providersTable.classList.remove("hidden");
        providers.forEach(function (p) {
            const tr = document.createElement("tr");
            const masked = p.apiKeyMasked ? " · Key: " + p.apiKeyMasked : "";
            tr.innerHTML =
                "<td>" + escapeHtml(p.displayName) + masked + "</td>" +
                "<td>" + escapeHtml(p.providerType) + "</td>" +
                "<td>" + escapeHtml(p.baseUrl || "-") + "</td>" +
                "<td>" + escapeHtml(p.defaultLlmModel || "-") + (p.defaultLlm ? " ★" : "") + "</td>" +
                "<td>" + escapeHtml(p.defaultEmbeddingModel || "-") + (p.defaultEmbedding ? " ★" : "") + "</td>" +
                "<td>" + formatEnabled(p) + " · " + formatTest(p) + "</td>" +
                "<td class=\"action-cell\"></td>";
            const actions = tr.querySelector(".action-cell");
            actions.appendChild(actionButton("测试连接", function () { testProvider(p.id); }));
            actions.appendChild(actionButton("设为默认问答模型", function () { setDefault(p.id, "llm"); }));
            actions.appendChild(actionButton("设为默认向量模型", function () { setDefault(p.id, "embedding"); }));
            actions.appendChild(actionButton("编辑", function () { openForm(p); }));
            if (p.enabled) {
                actions.appendChild(actionButton("禁用", function () { toggleEnable(p.id, false); }));
            } else {
                actions.appendChild(actionButton("启用", function () { toggleEnable(p.id, true); }));
            }
            providersBody.appendChild(tr);
        });
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
        try {
            const response = await fetch("/model-providers/" + id + "/test", { method: "POST" });
            const payload = await response.json();
            alert(payload.message || (payload.status === "SUCCESS" ? "连接正常" : "连接异常"));
            await loadAll();
        } catch (e) {
            alert("测试连接失败");
        }
    }

    async function setDefault(id, kind) {
        const path = kind === "llm" ? "set-default-llm" : "set-default-embedding";
        try {
            const response = await fetch("/model-providers/" + id + "/" + path, { method: "POST" });
            const payload = await response.json();
            if (!response.ok) {
                alert(payload.message || "设置失败");
                return;
            }
            await loadAll();
        } catch (e) {
            alert("设置失败");
        }
    }

    async function toggleEnable(id, enable) {
        const path = enable ? "enable" : "disable";
        try {
            const response = await fetch("/model-providers/" + id + "/" + path, { method: "POST" });
            const payload = await response.json();
            if (!response.ok) {
                alert(payload.message || "操作失败");
                return;
            }
            await loadAll();
        } catch (e) {
            alert("操作失败");
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

    function clearError() {
        errorStatus.classList.add("hidden");
        errorMessage.textContent = "";
    }

    function showError(msg) {
        errorMessage.textContent = msg;
        errorStatus.classList.remove("hidden");
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
