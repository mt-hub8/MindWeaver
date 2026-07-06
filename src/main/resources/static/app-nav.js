(function () {
    var NAV_ITEMS = [
        { key: "overview", nav: "overview", href: "/index.html", label: "总览", icon: "◆" },
        { key: "documents", nav: "documents", href: "/documents.html", label: "文档管理", icon: "▤" },
        { key: "batch-ingestion", nav: "batch-ingestion", href: "/batch-ingestion.html", label: "批量导入", icon: "▥" },
        { key: "collections", nav: "collections", href: "/collections.html", label: "知识库分组", icon: "▦" },
        { key: "ask", nav: "ask", href: "/ask.html", label: "知识库问答", icon: "◎" },
        { key: "agent-tasks", nav: "agent-tasks", href: "/agent-tasks.html", label: "AI 任务", icon: "✦" },
        { key: "model-settings", nav: "model-settings", href: "/model-settings.html", label: "模型设置", icon: "⚙" },
        { key: "quality", nav: "quality", href: "/quality.html", label: "质量诊断", icon: "◇" },
        { key: "knowledge-health", nav: "knowledge-health", href: "/knowledge-health.html", label: "知识库体检", icon: "♥" },
        { key: "retrieval-settings", nav: "retrieval-settings", href: "/retrieval-settings.html", label: "检索设置", icon: "⛓" },
        { key: "trash", nav: "trash", href: "/trash.html", label: "垃圾箱", icon: "⌫" },
        { key: "notifications", nav: "notifications", href: "/notifications.html", label: "通知中心", icon: "🔔" },
        { key: "settings", nav: "settings", href: "/settings.html", label: "系统设置", icon: "◈" },
        { key: "guide", nav: "guide", href: "/guide.html", label: "使用指南", icon: "?" }
    ];

    var LEGACY_ACTIVE_MAP = {
        home: "overview"
    };

    function resolvePageKey(mount) {
        var fromBody = document.body.getAttribute("data-page");
        if (fromBody) {
            return fromBody;
        }
        var active = mount.getAttribute("data-active") || "";
        return LEGACY_ACTIVE_MAP[active] || active || "overview";
    }

    function init() {
        document.body.classList.add("mw-page");

        var sidebarMount = document.getElementById("app-sidebar-mount");
        if (!sidebarMount) {
            migrateLegacyPage();
            sidebarMount = document.getElementById("app-sidebar-mount");
        }
        if (!sidebarMount) {
            return;
        }

        var pageKey = resolvePageKey(sidebarMount);
        document.body.setAttribute("data-page", pageKey);

        var shell = sidebarMount.closest(".mw-app-shell, .app-shell");
        if (shell && !shell.classList.contains("mw-app-shell")) {
            shell.classList.add("mw-app-shell");
        }

        var mainPanel = document.querySelector(".mw-main-panel, .main-panel");
        if (mainPanel && !mainPanel.classList.contains("mw-main-panel")) {
            mainPanel.classList.add("mw-main-panel");
        }

        var existingNav = sidebarMount.querySelector(".mw-nav");
        if (existingNav) {
            sidebarMount.classList.add("mw-sidebar");
            syncActiveNav(pageKey);
            loadRuntimeMode();
            return;
        }

        sidebarMount.className = "mw-sidebar";
        sidebarMount.setAttribute("aria-label", "主导航");
        sidebarMount.innerHTML = buildSidebarHtml(pageKey);
        loadRuntimeMode();
    }

    function syncActiveNav(pageKey) {
        var items = document.querySelectorAll(".mw-nav-item");
        items.forEach(function (item) {
            var nav = item.getAttribute("data-nav");
            var isActive = nav === pageKey;
            item.classList.toggle("active", isActive);
            if (isActive) {
                item.setAttribute("aria-current", "page");
            } else {
                item.removeAttribute("aria-current");
            }
        });
    }

    function migrateLegacyPage() {
        var legacyMount = document.getElementById("app-nav-mount");
        if (!legacyMount) {
            return;
        }
        var page = legacyMount.closest(".page");
        if (!page) {
            return;
        }
        var activeLegacy = legacyMount.getAttribute("data-active") || "";
        var pageKey = LEGACY_ACTIVE_MAP[activeLegacy] || activeLegacy || "overview";
        document.body.classList.add("mw-page");
        document.body.setAttribute("data-page", pageKey);

        var shell = document.createElement("div");
        shell.className = "mw-app-shell app-shell";

        var sidebar = document.createElement("aside");
        sidebar.id = "app-sidebar-mount";
        sidebar.className = "mw-sidebar";
        sidebar.setAttribute("data-active", activeLegacy);
        sidebar.setAttribute("aria-label", "主导航");
        sidebar.innerHTML = buildSidebarHtml(pageKey);

        var main = document.createElement("main");
        main.className = "mw-main-panel main-panel";

        var container = document.createElement("div");
        container.className = "mw-page-content content-container";

        Array.prototype.slice.call(page.childNodes).forEach(function (node) {
            if (node === legacyMount) {
                return;
            }
            if (node.nodeType === Node.ELEMENT_NODE && node.tagName === "SCRIPT"
                && node.src && node.src.indexOf("app-nav.js") >= 0) {
                return;
            }
            container.appendChild(node);
        });

        main.appendChild(container);
        shell.appendChild(sidebar);
        shell.appendChild(main);
        page.replaceWith(shell);
    }

    function buildSidebarHtml(pageKey) {
        var brand =
            '<div class="mw-brand"><a href="/index.html">' +
            '<div class="mw-brand-mark" aria-hidden="true">◇</div>' +
            "<div>" +
            '<div class="mw-brand-name">MindWeaver</div>' +
            '<div class="mw-brand-subtitle">AI Knowledge Workspace</div>' +
            "</div></a></div>";

        var dots =
            '<div class="mw-window-dots" aria-hidden="true">' +
            '<span class="dot red"></span><span class="dot yellow"></span><span class="dot green"></span>' +
            "</div>";

        var links = NAV_ITEMS.map(function (item) {
            var isActive = item.nav === pageKey;
            var cls = "mw-nav-item" + (isActive ? " active" : "");
            var aria = isActive ? ' aria-current="page"' : "";
            return (
                '<a class="' + cls + '" data-nav="' + item.nav + '" href="' + item.href + '"' + aria + ">" +
                '<span class="mw-nav-icon" aria-hidden="true">' + item.icon + "</span>" +
                "<span>" + item.label + "</span></a>"
            );
        }).join("");

        return (
            dots + brand +
            '<nav class="mw-nav">' + links + "</nav>" +
            '<div class="mw-workspace-card">' +
            '<div class="workspace-avatar" aria-hidden="true">M</div>' +
            '<div class="workspace-meta">' +
            '<div class="workspace-title">MindWeaver Workspace</div>' +
            '<div class="workspace-subtitle" id="sidebar-runtime-mode">本地知识库</div>' +
            "</div>" +
            '<div class="workspace-arrow" aria-hidden="true">›</div>' +
            "</div>"
        );
    }

    function loadRuntimeMode() {
        var el = document.getElementById("sidebar-runtime-mode");
        if (!el) {
            return;
        }
        fetch("/runtime/status")
            .then(function (res) { return res.ok ? res.json() : null; })
            .then(function (data) {
                if (!data) {
                    el.textContent = "本地知识库";
                    return;
                }
                var profile = data.activeProfile || "default";
                if (profile === "default") {
                    el.textContent = "mock · 本地知识库";
                } else if (profile.indexOf("local") >= 0) {
                    el.textContent = "local-ai · 本地知识库";
                } else {
                    el.textContent = profile + " · 本地知识库";
                }
            })
            .catch(function () {
                el.textContent = "本地知识库";
            });
    }

    if (document.readyState === "loading") {
        document.addEventListener("DOMContentLoaded", init);
    } else {
        init();
    }
})();
