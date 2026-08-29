(function () {
    var METRIC_LABELS = {
        RECALL_AT_K: "Recall@K（召回率）",
        HIT_RATE_AT_K: "HitRate@K（命中率）",
        MRR_AT_K: "MRR@K（平均倒数排名）",
        NDCG_AT_K: "NDCG@K（归一化折损累计增益）",
        CONTEXT_PRECISION_AT_K: "ContextPrecision@K（上下文精确率）",
        CROSS_COLLECTION_LEAK_RATE: "CrossCollectionLeakRate（跨集合污染率）",
        WRONG_VERSION_LEAK_RATE: "WrongVersionLeakRate（错误版本污染率）",
        FAITHFULNESS: "Faithfulness（忠实性）",
        CITATION_ACCURACY: "CitationAccuracy（引用准确率）",
        ANSWER_COVERAGE: "AnswerCoverage（答案覆盖率）",
        REFUSAL_ACCURACY: "RefusalAccuracy（拒答准确率）"
    };

    function $(id) { return document.getElementById(id); }

    function fetchJson(url, options) {
        return fetch(url, options).then(function (res) {
            if (!res.ok) {
                return res.json().then(function (body) {
                    throw new Error(body.message || ("HTTP " + res.status));
                }).catch(function () {
                    throw new Error("HTTP " + res.status);
                });
            }
            return res.json();
        });
    }

    function loadDatasets() {
        return fetchJson("/rag/evaluation/datasets").then(function (datasets) {
            var select = $("dataset-select");
            select.innerHTML = '<option value="">请选择评测集</option>';
            datasets.forEach(function (ds) {
                var opt = document.createElement("option");
                opt.value = ds.datasetId;
                opt.textContent = ds.name + "（" + (ds.caseCount || 0) + " cases）";
                select.appendChild(opt);
            });
        });
    }

    function loadCollections() {
        return fetchJson("/collections").then(function (collections) {
            var select = $("collection-select");
            select.innerHTML = '<option value="">不指定</option>';
            collections.forEach(function (c) {
                var opt = document.createElement("option");
                opt.value = c.collectionId;
                opt.textContent = c.name;
                select.appendChild(opt);
            });
        }).catch(function () { /* optional */ });
    }

    function showRunResult(run) {
        $("score-card").classList.remove("hidden");
        $("metrics-card").classList.remove("hidden");
        $("issues-card").classList.remove("hidden");
        $("suggestions-card").classList.remove("hidden");
        $("cases-card").classList.remove("hidden");

        $("overall-score").textContent = run.overallScore == null ? "—" : run.overallScore;
        $("overall-level").textContent = run.overallScoreLevel || "—";
        $("run-profile-label").textContent = run.scoringProfileDisplayName || "—";
        $("run-strategy-label").textContent = run.strategyDisplayName || "—";
        $("run-dataset-label").textContent = "评测集 #" + run.datasetId;

        renderMetrics(run.summary);
        renderDiagnosis(run.diagnosis);
        $("tech-json").textContent = JSON.stringify(run, null, 2);

        fetchJson("/rag/evaluation/runs/" + run.runId + "/cases").then(renderCaseResults);
        refreshStrategyCompare(run);
    }

    function renderMetrics(summary) {
        var grid = $("metrics-grid");
        grid.innerHTML = "";
        if (!summary) return;
        var metrics = []
            .concat(summary.retrievalMetrics || [])
            .concat(summary.generationMetrics || []);
        metrics.forEach(function (m) {
            var card = document.createElement("div");
            card.className = "mw-stat-card summary-stat";
            var label = METRIC_LABELS[m.code] || m.displayName || m.code;
            var value = m.available === false ? "不可用" : (m.displayValue || "—");
            var reason = m.unavailableReason ? '<span class="muted" style="font-size:0.78rem">' + m.unavailableReason + "</span>" : "";
            card.innerHTML = '<span class="mw-stat-label">' + label + "</span>" +
                '<span class="mw-stat-value">' + value + "</span>" + reason;
            grid.appendChild(card);
        });
    }

    function renderDiagnosis(diagnosis) {
        var issues = (diagnosis && diagnosis.issues) || [];
        var suggestions = (diagnosis && diagnosis.suggestions) || [];
        var issuesList = $("issues-list");
        var suggestionsList = $("suggestions-list");
        issuesList.innerHTML = "";
        suggestionsList.innerHTML = "";
        issues.forEach(function (issue) {
            var li = document.createElement("li");
            li.textContent = "[" + (issue.severity || "—") + "] " + issue.title + " — " + issue.description;
            issuesList.appendChild(li);
        });
        suggestions.forEach(function (s) {
            var li = document.createElement("li");
            li.textContent = s.title + "（" + (s.actionType || "") + " / " + (s.priority || "") + "）— " + s.description;
            suggestionsList.appendChild(li);
        });
        $("issues-empty").classList.toggle("hidden", issues.length > 0);
        $("suggestions-empty").classList.toggle("hidden", suggestions.length > 0);
    }

    function renderCaseResults(cases) {
        var container = $("case-results-list");
        container.innerHTML = "";
        cases.forEach(function (c) {
            var div = document.createElement("div");
            div.className = "entity-item";
            var leak = (c.retrievedChunks || []).some(function (ch) { return ch.wrongCollection; });
            var versionLeak = (c.retrievedChunks || []).some(function (ch) { return ch.wrongVersion; });
            div.innerHTML = "<strong>" + escapeHtml(c.query) + "</strong>" +
                "<p class='muted'>score: " + (c.qualityScore == null ? "—" : c.qualityScore) +
                " · 跨集合污染: " + (leak ? "是" : "否") +
                " · 错误版本污染: " + (versionLeak ? "是" : "否") + "</p>" +
                "<details><summary>查看详情</summary><pre class='muted'>" + escapeHtml(JSON.stringify(c, null, 2)) + "</pre></details>";
            container.appendChild(div);
        });
    }

    function refreshStrategyCompare(currentRun) {
        fetchJson("/rag/evaluation/runs").then(function (runs) {
            var sameDataset = runs.filter(function (r) {
                return r.datasetId === currentRun.datasetId && r.status === "COMPLETED";
            });
            if (sameDataset.length < 2) {
                $("strategy-compare-hint").classList.remove("hidden");
                $("strategy-compare-list").classList.add("hidden");
                return;
            }
            $("strategy-compare-hint").classList.add("hidden");
            $("strategy-compare-list").classList.remove("hidden");
            var list = $("strategy-compare-list");
            list.innerHTML = "";
            sameDataset.forEach(function (r) {
                var item = document.createElement("div");
                item.className = "entity-item";
                item.innerHTML = "<strong>" + (r.strategyDisplayName || r.strategy) + "</strong>" +
                    "<p class='muted'>评分 " + (r.overallScore == null ? "—" : r.overallScore) + " / 100 · " + (r.overallScoreLevel || "") + "</p>";
                list.appendChild(item);
            });
        });
    }

    function escapeHtml(text) {
        return String(text).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
    }

    function runEvaluation() {
        var datasetId = $("dataset-select").value;
        if (!datasetId) {
            alert("请先选择评测集");
            return;
        }
        var payload = {
            datasetId: Number(datasetId),
            name: "知识库体检 " + new Date().toLocaleString(),
            strategy: $("strategy-select").value,
            scoringProfile: $("profile-select").value,
            topK: Number($("topk-input").value || 5),
            executeGeneration: $("execute-generation").value === "true"
        };
        var collectionId = $("collection-select").value;
        if (collectionId) payload.collectionId = Number(collectionId);

        $("run-status").classList.remove("hidden");
        $("run-eval-btn").disabled = true;
        fetchJson("/rag/evaluation/runs", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(payload)
        }).then(function (run) {
            showRunResult(run);
        }).catch(function (err) {
            alert("评测失败：" + err.message);
        }).finally(function () {
            $("run-status").classList.add("hidden");
            $("run-eval-btn").disabled = false;
        });
    }

    document.addEventListener("DOMContentLoaded", function () {
        loadDatasets();
        loadCollections();
        $("run-eval-btn").addEventListener("click", runEvaluation);
    });
})();
