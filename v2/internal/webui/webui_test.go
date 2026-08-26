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
		`/api/v1/ollama/probe`, `method: "PUT"`, `/api/v1/conversations/messages`, `/api/v1/answers?id=`,
		`"Idempotency-Key": attempt.key`, `body: attempt.body`, `answer.status === "pending"`,
		`answer.limitationCode === "OUTCOME_UNCERTAIN"`, `content.textContent = source.content`,
		`conversation.pendingAnswer`, `activeConversation.pendingAnswerId`, `askInFlight`, `askOutcomeUncertain`, `generation !== askGeneration`,
		`remove.dataset.conversationId`, `questionBytes > 1024`, "删除并解除引用",
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
		"const generation = ++conversationGeneration",
		"const generation = ++messageGeneration",
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
	if !strings.Contains(index.String(), `id="ask-question" maxlength="1024"`) || !strings.Contains(index.String(), "最多 1024 UTF-8 字节") {
		t.Fatal("embedded Ask form does not expose the search-compatible question byte limit")
	}
	for _, id := range []string{`id="active-documents"`, `id="trashed-documents"`, `id="documents-more"`, `id="collections"`, `id="collection-members"`, `id="purges"`, `id="ollama-form"`, `id="conversations"`, `id="ask-form"`, `id="messages"`} {
		if !strings.Contains(index.String(), id) {
			t.Fatalf("embedded shell is missing %s", id)
		}
	}
}

func TestEmbeddedClientResumesPendingAnswerPollingAfterConversationSelection(t *testing.T) {
	javascript, err := assets.ReadFile("static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	source := string(javascript)
	selectStart := strings.Index(source, "async function selectConversation")
	loadStart := strings.Index(source, "async function loadConversations")
	if selectStart < 0 || loadStart < 0 {
		t.Fatal("embedded conversation functions are not discoverable")
	}
	selectEnd := strings.Index(source[selectStart:], `byId("conversation-form")`)
	loadEnd := strings.Index(source[loadStart:], "function updateActiveConversationLabel")
	if selectEnd < 0 || loadEnd < 0 {
		t.Fatal("embedded conversation functions are not discoverable")
	}
	if !strings.Contains(source[selectStart:selectStart+selectEnd], "syncActiveConversationPoll()") ||
		!strings.Contains(source[loadStart:loadStart+loadEnd], "syncActiveConversationPoll()") ||
		!strings.Contains(source, "startAnswerPoll(activeConversation.pendingAnswerId, activeConversation.id)") ||
		!strings.Contains(source, `/api/v1/answers?id=${encodeURIComponent(answerID)}`) {
		t.Fatal("pending conversation selection/refresh does not resume answer-ID polling")
	}
}

func TestEmbeddedClientDoesNotClaimSemanticCitationVerification(t *testing.T) {
	javascript, err := assets.ReadFile("static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	source := string(javascript)
	for _, overclaim := range []string{"验证引用", "引用已验证", "语义验证", "事实核验"} {
		if strings.Contains(source, overclaim) {
			t.Fatalf("embedded client overclaims citation assurance with %q", overclaim)
		}
	}
	for _, honestBoundary := range []string{
		"只会发布指向本次所用资料的结构化引用",
		"引用编号均指向本次所用资料",
	} {
		if !strings.Contains(source, honestBoundary) {
			t.Fatalf("embedded client is missing structural citation boundary %q", honestBoundary)
		}
	}
}
