(function () {
    const activeProfile = document.getElementById("settings-active-profile");
    const vectorStore = document.getElementById("settings-vector-store");
    const pythonWorker = document.getElementById("settings-python-worker");
    const ollama = document.getElementById("settings-ollama");
    const projectPath = document.getElementById("settings-project-path");

    projectPath.textContent = "见启动目录（user.dir）";

    loadRuntimeSummary();

    async function loadRuntimeSummary() {
        try {
            const response = await fetch("/runtime/status");
            const data = await response.json();
            if (!response.ok) {
                activeProfile.textContent = "无法加载";
                return;
            }
            activeProfile.textContent = data.activeProfile === "default"
                ? "default（默认 / mock 或项目配置）"
                : (data.activeProfile || "-");
            vectorStore.textContent = data.vectorStoreProvider || "-";
            pythonWorker.textContent = data.pythonWorkerBaseUrl || "-";
            ollama.textContent = data.ollamaBaseUrl || "-";
        } catch (error) {
            activeProfile.textContent = "服务未启动";
        }
    }
})();
