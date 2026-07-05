(function () {
    const loadingStatus = document.getElementById("loading-status");
    const errorStatus = document.getElementById("error-status");
    const errorMessage = document.getElementById("error-message");
    const statusPanel = document.getElementById("status-panel");
    const connectionPanel = document.getElementById("connection-panel");
    const statusMessage = document.getElementById("status-message");
    const testResult = document.getElementById("test-result");

    const fields = {
        activeProfile: document.getElementById("active-profile"),
        embeddingProvider: document.getElementById("embedding-provider"),
        embeddingModel: document.getElementById("embedding-model"),
        embeddingDimension: document.getElementById("embedding-dimension"),
        llmProvider: document.getElementById("llm-provider"),
        llmModel: document.getElementById("llm-model"),
        vectorStoreProvider: document.getElementById("vector-store-provider"),
        pythonWorkerUrl: document.getElementById("python-worker-url"),
        pythonWorkerStatus: document.getElementById("python-worker-status"),
        ollamaUrl: document.getElementById("ollama-url"),
        ollamaStatus: document.getElementById("ollama-status")
    };

    document.getElementById("refresh-status-button").addEventListener("click", loadStatus);
    document.getElementById("test-embedding-button").addEventListener("click", function () {
        runTest("/runtime/test/embedding", "Embedding");
    });
    document.getElementById("test-llm-button").addEventListener("click", function () {
        runTest("/runtime/test/llm", "LLM");
    });

    loadStatus();

    async function loadStatus() {
        setLoading(true);
        clearError();
        try {
            const response = await fetch("/runtime/status");
            const payload = await response.json();
            if (!response.ok) {
                showError("无法加载运行时状态（HTTP " + response.status + "）");
                return;
            }
            renderStatus(payload);
        } catch (error) {
            showError("无法连接服务，请确认应用已启动。");
        } finally {
            setLoading(false);
        }
    }

    function renderStatus(data) {
        statusPanel.classList.remove("hidden");
        connectionPanel.classList.remove("hidden");
        statusMessage.textContent = data.statusMessage || "";

        fields.activeProfile.textContent = formatProfile(data.activeProfile);
        fields.embeddingProvider.textContent = data.embeddingProvider || "-";
        fields.embeddingModel.textContent = data.embeddingModel || "-";
        fields.embeddingDimension.textContent = data.embeddingDimension != null ? String(data.embeddingDimension) : "-";
        fields.llmProvider.textContent = data.llmProvider || "-";
        fields.llmModel.textContent = data.llmModel || "-";
        fields.vectorStoreProvider.textContent = data.vectorStoreProvider || "-";
        fields.pythonWorkerUrl.textContent = data.pythonWorkerBaseUrl || "-";
        fields.ollamaUrl.textContent = data.ollamaBaseUrl || "-";
        fields.pythonWorkerStatus.textContent = formatReachable(data.pythonWorkerReachable);
        fields.ollamaStatus.textContent = formatReachable(data.ollamaReachable);
    }

    async function runTest(path, label) {
        testResult.classList.remove("hidden");
        testResult.textContent = "正在测试 " + label + " 连接…";
        try {
            const response = await fetch(path, { method: "POST" });
            const payload = await response.json();
            if (payload.success) {
                testResult.textContent =
                    label + "：连接正常（" + (payload.latencyMs || "-") + " ms）" +
                    (payload.model ? " · 模型 " + payload.model : "");
            } else {
                testResult.textContent = label + "：连接异常 — " + (payload.message || "未知错误");
            }
        } catch (error) {
            testResult.textContent = label + "：连接异常 — 请求失败";
        }
    }

    function formatProfile(profile) {
        if (!profile || profile === "default") {
            return "default（默认）";
        }
        return profile;
    }

    function formatReachable(value) {
        if (value === true) {
            return "连接正常";
        }
        if (value === false) {
            return "连接异常";
        }
        return "未检测";
    }

    function setLoading(isLoading) {
        loadingStatus.classList.toggle("hidden", !isLoading);
        loadingStatus.classList.toggle("visible", isLoading);
    }

    function clearError() {
        errorStatus.classList.add("hidden");
        errorMessage.textContent = "";
    }

    function showError(message) {
        errorMessage.textContent = message;
        errorStatus.classList.remove("hidden");
    }
})();
