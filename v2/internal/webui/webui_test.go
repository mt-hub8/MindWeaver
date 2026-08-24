package webui

import (
	"bytes"
	"strings"
	"testing"
)

func TestEmbeddedClientRefreshesCSRFAndConsumesBoundedSearchPages(t *testing.T) {
	javascript, err := assets.ReadFile("static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	source := string(javascript)
	refreshAt := strings.Index(source, `fetch("/session/csrf"`)
	runtimeAt := strings.Index(source, `api("/api/v1/runtime")`)
	if refreshAt < 0 || runtimeAt < 0 || refreshAt > runtimeAt {
		t.Fatalf("CSRF refresh/runtime wiring positions = %d/%d", refreshAt, runtimeAt)
	}
	for _, contract := range []string{`credentials: "same-origin"`, "csrfToken = await refreshCSRF()", "payload.nextOffset", "hit.snippet", "hit.snippetTruncated", "generation !== searchGeneration"} {
		if !strings.Contains(source, contract) {
			t.Fatalf("embedded JavaScript is missing %q", contract)
		}
	}
	if strings.Contains(source, "hit.content") {
		t.Fatal("embedded JavaScript still consumes unbounded hit.content")
	}
	for _, contract := range []string{
		`expectedRevision: documentItem.revision`, `"Idempotency-Key": collectionKey`,
		`method: "DELETE"`, `payload.nextCursor`, `/api/v1/documents/purge-status`,
		`payload.complete === true && payload.state === "complete"`, "无进行中的清理", "重试清理",
		`/api/v1/documents/retry-ingestion`, `documentItem.ingestionErrorCode`, "重试摄取", "取消摄取",
		`const maxUploadBytes = 4 * 1024 * 1024`, `payload.sweptBlobCandidates`,
	} {
		if !strings.Contains(source, contract) {
			t.Fatalf("embedded product workflow is missing %q", contract)
		}
	}
	if strings.Contains(source, "/api/v1/documents?limit=100") {
		t.Fatal("embedded JavaScript still silently truncates the document catalog")
	}
	for _, generation := range []string{
		"const generation = ++documentGeneration",
		"const generation = ++collectionGeneration",
		"const generation = ++memberGeneration",
		"const generation = ++purgeGeneration",
	} {
		if !strings.Contains(source, generation) {
			t.Fatalf("embedded catalog lacks monotonic in-flight guard %q", generation)
		}
	}
	if strings.Contains(source, "reset ? ++documentGeneration") || strings.Contains(source, "reset ? ++collectionGeneration") ||
		strings.Contains(source, "reset ? ++memberGeneration") || strings.Contains(source, "reset ? ++purgeGeneration") {
		t.Fatal("append requests can still share a generation and apply a stale cursor response")
	}

	var index bytes.Buffer
	if err := indexTemplate.Execute(&index, struct{ CSS, JS string }{CSS: "/assets/test.css", JS: "/assets/test.js"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(index.String(), `id="search-more"`) || !strings.Contains(index.String(), "hidden>加载更多结果") {
		t.Fatal("embedded shell does not expose a hidden continuation control")
	}
	for _, id := range []string{`id="active-documents"`, `id="trashed-documents"`, `id="documents-more"`, `id="collections"`, `id="collection-members"`, `id="purges"`} {
		if !strings.Contains(index.String(), id) {
			t.Fatalf("embedded shell is missing %s", id)
		}
	}
}
