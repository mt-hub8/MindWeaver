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
		`expectedRevision: documentItem.revision`, `"Idempotency-Key": attempt.key`,
		`method: "DELETE"`, `payload.nextCursor`, `/api/v1/documents/purge-status`,
		`payload.complete === true && payload.state === "complete"`, "无进行中的清理", "重试清理",
		`/api/v1/documents/retry-ingestion`, `documentItem.ingestionErrorCode`, "重试摄取", "取消摄取",
		`const maxUploadBytes = 4 * 1024 * 1024`, `payload.sweptBlobCandidates`,
		`/api/v1/ollama/probe`, `method: "PUT"`, `/api/v1/conversations/messages`, `/api/v1/answers?id=`,
		`"Idempotency-Key": attempt.key`, `body: attempt.body`, `answer.status === "pending"`,
		`answer.limitationCode === "OUTCOME_UNCERTAIN"`, `content.textContent = source.content`,
		`conversation.pendingAnswer`, `activeConversation.pendingAnswerId`, `askInFlight`, `askOutcomeUncertain`, `generation !== askGeneration`,
		`remove.dataset.conversationId`, `questionBytes > 1024`, "删除并解除引用",
		`api("/api/v1/backups"`, `/api/v1/backups/status?operationId=`, `api("/api/v1/backups/cancel"`,
		`"Idempotency-Key": attempt.key`, `JSON.stringify({ destination: attempt.destination })`,
		`if (terminal) backupAttempt = null`, `status.state === "needs_attention"`,
		`byId("backup-destination").disabled = !terminal`,
		`byId("backup-destination").disabled = true`,
		`byId("backup-destination").value = backupAttempt.destination`,
		`if (!showBackupStatus(status)) await pollBackup(attempt.operationID, generation)`,
		`if (!attempt || attempt.operationID !== operationID || attempt.polling) return`,
		`backupAttempt !== attempt || attempt.operationID !== operationID`, `attempt.polling = false`,
		"如需重试，请重新提交相同目标，新请求会先检查受控暂存残留",
		"无法证明它由本次新请求创建；未将其冒充为新备份",
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
	if !strings.Contains(index.String(), "备份包是未加密的明文 SQLite 与资料对象") ||
		!strings.Contains(index.String(), "磁盘加密和访问控制") {
		t.Fatal("embedded backup workflow does not disclose its plaintext boundary")
	}
	for _, id := range []string{`id="active-documents"`, `id="trashed-documents"`, `id="documents-more"`, `id="collections"`, `id="collection-members"`, `id="purges"`, `id="ollama-form"`, `id="conversations"`, `id="ask-form"`, `id="messages"`, `id="backup-form"`, `id="backup-destination"`, `id="backup-cancel"`} {
		if !strings.Contains(index.String(), id) {
			t.Fatalf("embedded shell is missing %s", id)
		}
	}
}

func TestEmbeddedClientBindsImmutableMutationAttemptsAndFreezesAskReplay(t *testing.T) {
	javascript, err := assets.ReadFile("static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	source := string(javascript)
	for _, legacy := range []string{"uploadKey", "collectionKey", "conversationKey"} {
		if strings.Contains(source, legacy) {
			t.Fatalf("embedded client retains legacy unbound key %q", legacy)
		}
	}
	for _, contract := range []string{
		"function definiteMutationFailure(error)",
		"return Number.isInteger(error.status) && error.status < 500;",
		`byId("ask-question").disabled = replayLocked`,
		`byId("ask-collection").disabled = replayLocked`,
		`select.dataset.conversationSelectId = conversation.id`,
		`if (askReplayLocked()) {`,
		`if (conversationInFlight || askReplayLocked()) {`,
	} {
		if !strings.Contains(source, contract) {
			t.Fatalf("embedded client is missing mutation-attempt contract %q", contract)
		}
	}

	upload := javascriptSection(t, source,
		`function updateUploadAttemptAvailability()`, `async function loadSearchPage`)
	assertJSContracts(t, "upload", upload, []string{
		`uploadAttempt = Object.freeze({`,
		`const attempt = uploadAttempt`,
		`"Idempotency-Key": attempt.key`,
		`encodeTextHeader(attempt.title)`,
		`encodeTextHeader(attempt.filename)`,
		`body: attempt.file`,
		`if (!admitted && definiteMutationFailure(error)) clearUploadAttempt()`,
		`uploadAttempt === attempt`,
	})
	assertFrozenSendReadsNoLiveInput(t, "upload", upload, `const attempt = uploadAttempt`,
		[]string{`byId("title").value`, `byId("file").files[0]`})

	collection := javascriptSection(t, source,
		`function updateCollectionAttemptAvailability()`, `async function loadMembers`)
	assertJSContracts(t, "collection", collection, []string{
		`collectionAttempt = Object.freeze({`,
		`body: JSON.stringify({ name })`,
		`const attempt = collectionAttempt`,
		`"Idempotency-Key": attempt.key`,
		`body: attempt.body`,
		`const collection = payload && payload.collection`,
		`throw new Error("本地服务返回了无效的集合结果。")`,
		`if (definiteMutationFailure(error)) clearCollectionAttempt()`,
		`collectionAttempt === attempt`,
	})
	assertFrozenSendReadsNoLiveInput(t, "collection", collection, `const attempt = collectionAttempt`,
		[]string{`byId("collection-name").value`, `JSON.stringify({ name: byId(`})
	assertJSOrder(t, "collection", collection, `const attempt = collectionAttempt`,
		`const collection = payload && payload.collection`, `clearCollectionAttempt();`)

	conversation := javascriptSection(t, source,
		`function clearConversationAttempt()`, `async function deleteConversation`)
	assertJSContracts(t, "conversation", conversation, []string{
		`conversationAttempt = Object.freeze({`,
		`body: JSON.stringify({ title })`,
		`const attempt = conversationAttempt`,
		`"Idempotency-Key": attempt.key`,
		`body: attempt.body`,
		`const conversation = payload && payload.conversation`,
		`throw new Error("本地服务返回了无效的会话结果。")`,
		`if (definiteMutationFailure(error)) clearConversationAttempt()`,
		`conversationAttempt === attempt`,
	})
	assertFrozenSendReadsNoLiveInput(t, "conversation", conversation, `const attempt = conversationAttempt`,
		[]string{`body: JSON.stringify({ title: byId(`})
	assertJSOrder(t, "conversation", conversation, `const attempt = conversationAttempt`,
		`const conversation = payload && payload.conversation`, `clearConversationAttempt();`)

	askControls := javascriptSection(t, source,
		`function askReplayLocked()`, `async function loadOllamaConfiguration`)
	ask := javascriptSection(t, source,
		`byId("ask-form").addEventListener`, `byId("documents-more").addEventListener`)
	assertJSContracts(t, "Ask", ask, []string{
		`askAttempt = Object.freeze({`,
		`body: JSON.stringify(requestBody)`,
		`question,`,
		`scope`,
		`const attempt = askAttempt`,
		`restoreAskAttemptInputs(attempt)`,
		`"Idempotency-Key": attempt.key`,
		`body: attempt.body`,
		`askOutcomeUncertain = true`,
	})
	if !strings.Contains(askControls, `askAttempt !== null && (askInFlight || askOutcomeUncertain ||`) {
		t.Fatal("Ask visible-input lock is not bound to the retained uncertain attempt")
	}
	assertFrozenSendReadsNoLiveInput(t, "Ask", ask, `const attempt = askAttempt`,
		[]string{`byId("ask-question").value`, `byId("ask-collection").value.trim()`})

	for name, section := range map[string]string{
		"upload": upload, "collection": collection, "conversation": conversation, "Ask": ask,
	} {
		if count := strings.Count(section, "crypto.randomUUID()"); count != 1 {
			t.Fatalf("%s attempt creates %d idempotency keys, want exactly one", name, count)
		}
	}
}

func javascriptSection(t *testing.T, source, start, end string) string {
	t.Helper()
	startAt := strings.Index(source, start)
	if startAt < 0 {
		t.Fatalf("JavaScript section start %q is missing", start)
	}
	endAt := strings.Index(source[startAt:], end)
	if endAt < 0 {
		t.Fatalf("JavaScript section end %q is missing after %q", end, start)
	}
	return source[startAt : startAt+endAt]
}

func assertJSContracts(t *testing.T, name, section string, contracts []string) {
	t.Helper()
	for _, contract := range contracts {
		if !strings.Contains(section, contract) {
			t.Fatalf("%s attempt is missing %q", name, contract)
		}
	}
}

func assertFrozenSendReadsNoLiveInput(t *testing.T, name, section, sendStart string, forbidden []string) {
	t.Helper()
	startAt := strings.Index(section, sendStart)
	if startAt < 0 {
		t.Fatalf("%s send boundary %q is missing", name, sendStart)
	}
	send := section[startAt:]
	for _, value := range forbidden {
		if strings.Contains(send, value) {
			t.Fatalf("%s send still reads live input %q after freezing its attempt", name, value)
		}
	}
}

func assertJSOrder(t *testing.T, name, section string, contracts ...string) {
	t.Helper()
	position := 0
	for _, contract := range contracts {
		next := strings.Index(section[position:], contract)
		if next < 0 {
			t.Fatalf("%s attempt is missing ordered contract %q", name, contract)
		}
		position += next + len(contract)
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
