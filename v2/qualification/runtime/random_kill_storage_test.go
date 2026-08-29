package runtimequalification_test

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	rag "github.com/mt-hub8/MindWeaver/v2/internal/rag"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
)

type rel001StorageCounts struct {
	migrations, expectedMigrations                    int64
	collections, collectionDocuments                  int64
	documents, revisions, ingestions, pdfIngestions   int64
	jobs, succeededJobs, chunks, ftsRows              int64
	ollamaConfigs, activeOllama                       int64
	conversations, messages, askRequests              int64
	answerSources, answerCitations                    int64
	documentLifecycle, blobCandidates                 int64
	documentPurges, documentPurgeBlobs                int64
	obsoleteSettings, obsoleteTemporary, legacyTables int64
}

func assertREL001KillStorage(t *testing.T, runRoot string, frame rel001KillFrame, template rel001KillTemplate, identity rel001KillIdentity, conversationID, providerEndpoint string) {
	t.Helper()
	vaultRoot := filepath.Join(runRoot, "vault")
	database := openQualificationDatabase(t, filepath.Join(vaultRoot, "data", store.DatabaseFileName))
	assertDatabaseIntegrity(t, database)
	if err := store.CheckCanonicalConsistency(t.Context(), database); err != nil {
		t.Fatal("REL001_KILL_STORAGE_CANONICAL_CONSISTENCY_FAILED")
	}
	got := readREL001StorageCounts(t, database)
	want := rel001StorageCounts{migrations: 7, expectedMigrations: 7}
	switch frame.mutation {
	case rel001KillCollection:
		want.collections = 1
		assertREL001CollectionRows(t, database, template, identity)
	case rel001KillConversation:
		want.conversations = 1
		assertREL001ConversationRows(t, database, template, identity)
	case rel001KillUpload:
		want.documents, want.revisions, want.ingestions = 1, 1, 1
		want.jobs, want.succeededJobs, want.chunks, want.ftsRows = 1, 1, 1, 1
		assertREL001UploadRows(t, database, template, identity)
	case rel001KillAskNoContext:
		want.ollamaConfigs, want.activeOllama = 1, 1
		want.conversations, want.messages, want.askRequests = 1, 2, 1
		assertREL001AskRows(t, database, frame, template, identity, conversationID, providerEndpoint)
	default:
		t.Fatal("REL001_KILL_STORAGE_MUTATION_INVALID")
	}
	if got != want {
		t.Fatalf("REL001_KILL_STORAGE_COUNTS_INVALID mutation=%s got=%+v want=%+v", rel001KillMutationNames[frame.mutation], got, want)
	}
	if err := database.Close(); err != nil {
		t.Fatal("REL001_KILL_STORAGE_DATABASE_CLOSE_FAILED")
	}
	wantObjects := int64(0)
	if frame.mutation == rel001KillUpload {
		wantObjects = 1
	}
	assertREL001BlobTree(t, vaultRoot, template, wantObjects)
}

func readREL001StorageCounts(t *testing.T, database *sql.DB) rel001StorageCounts {
	t.Helper()
	var state rel001StorageCounts
	queries := []struct {
		destination *int64
		statement   string
	}{
		{&state.migrations, "SELECT count(*) FROM schema_migrations"},
		{&state.expectedMigrations, "SELECT count(*) FROM schema_migrations WHERE version IN (1, 2, 3, 4, 5, 6, 7)"},
		{&state.collections, "SELECT count(*) FROM collections"},
		{&state.collectionDocuments, "SELECT count(*) FROM collection_documents"},
		{&state.documents, "SELECT count(*) FROM documents"},
		{&state.revisions, "SELECT count(*) FROM document_revisions"},
		{&state.ingestions, "SELECT count(*) FROM document_ingestions"},
		{&state.pdfIngestions, "SELECT count(*) FROM document_ingestions WHERE source_format = 'pdf'"},
		{&state.jobs, "SELECT count(*) FROM jobs"},
		{&state.succeededJobs, "SELECT count(*) FROM jobs WHERE status = 'succeeded'"},
		{&state.chunks, "SELECT count(*) FROM chunks"},
		{&state.ftsRows, "SELECT count(*) FROM chunks_fts"},
		{&state.ollamaConfigs, "SELECT count(*) FROM ollama_config_versions"},
		{&state.activeOllama, "SELECT count(*) FROM ollama_config_versions WHERE is_active = 1"},
		{&state.conversations, "SELECT count(*) FROM conversations"},
		{&state.messages, "SELECT count(*) FROM conversation_messages"},
		{&state.askRequests, "SELECT count(*) FROM ask_requests"},
		{&state.answerSources, "SELECT count(*) FROM answer_sources"},
		{&state.answerCitations, "SELECT count(*) FROM answer_citations"},
		{&state.documentLifecycle, "SELECT count(*) FROM document_lifecycle"},
		{&state.blobCandidates, "SELECT count(*) FROM blob_gc_candidates"},
		{&state.documentPurges, "SELECT count(*) FROM document_purges"},
		{&state.documentPurgeBlobs, "SELECT count(*) FROM document_purge_blobs"},
		{&state.obsoleteSettings, "SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = 'settings'"},
		{&state.obsoleteTemporary, "SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = 'document_ingestions_pdf'"},
		{&state.legacyTables, "SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name IN ('legacy_imports', 'legacy_ollama_intents')"},
	}
	for _, query := range queries {
		if err := database.QueryRowContext(t.Context(), query.statement).Scan(query.destination); err != nil {
			t.Fatal("REL001_KILL_STORAGE_COUNT_QUERY_FAILED")
		}
	}
	return state
}

func assertREL001CollectionRows(t *testing.T, database *sql.DB, template rel001KillTemplate, identity rel001KillIdentity) {
	t.Helper()
	var id, name string
	if err := database.QueryRowContext(t.Context(), "SELECT id, name FROM collections").Scan(&id, &name); err != nil || id != identity.primary || name != template.expected {
		t.Fatal("REL001_KILL_COLLECTION_STORAGE_IDENTITY_INVALID")
	}
}

func assertREL001ConversationRows(t *testing.T, database *sql.DB, template rel001KillTemplate, identity rel001KillIdentity) {
	t.Helper()
	var id, title string
	var revision int64
	if err := database.QueryRowContext(t.Context(), "SELECT id, title, revision FROM conversations").Scan(&id, &title, &revision); err != nil ||
		id != identity.primary || title != template.expected || revision != 0 {
		t.Fatal("REL001_KILL_CONVERSATION_STORAGE_IDENTITY_INVALID")
	}
}

func assertREL001UploadRows(t *testing.T, database *sql.DB, template rel001KillTemplate, identity rel001KillIdentity) {
	t.Helper()
	wantBlob := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(template.body)))
	var documentID, title, mediaType, documentStatus string
	if err := database.QueryRowContext(t.Context(), "SELECT id, title, media_type, status FROM documents").Scan(&documentID, &title, &mediaType, &documentStatus); err != nil ||
		documentID != identity.primary || title != template.title || mediaType != "text/plain" || documentStatus != "active" {
		t.Fatal("REL001_KILL_UPLOAD_DOCUMENT_ROW_INVALID")
	}
	var revisionID, revisionDocumentID, contentHash, revisionBlob string
	var revisionNo int64
	var active int64
	if err := database.QueryRowContext(t.Context(), "SELECT id, document_id, revision_no, content_hash, source_blob_id, is_active FROM document_revisions").Scan(
		&revisionID, &revisionDocumentID, &revisionNo, &contentHash, &revisionBlob, &active); err != nil ||
		revisionID != identity.secondary || revisionDocumentID != identity.primary || revisionNo != 1 || contentHash != wantBlob[len("sha256:"):] || revisionBlob != wantBlob || active != 1 {
		t.Fatal("REL001_KILL_UPLOAD_REVISION_ROW_INVALID")
	}
	var ingestionKey, ingestionDocument, ingestionRevision, ingestionJob, ingestionBlob, filename, format string
	var sourceSize int64
	if err := database.QueryRowContext(t.Context(), `SELECT idempotency_key, document_id, revision_id, job_id, source_blob_id, source_size, source_filename, source_format FROM document_ingestions`).Scan(
		&ingestionKey, &ingestionDocument, &ingestionRevision, &ingestionJob, &ingestionBlob, &sourceSize, &filename, &format); err != nil ||
		ingestionKey != template.key || ingestionDocument != identity.primary || ingestionRevision != identity.secondary || ingestionJob != identity.tertiary ||
		ingestionBlob != wantBlob || sourceSize != int64(len(template.body)) || filename != template.filename || format != "text" {
		t.Fatal("REL001_KILL_UPLOAD_INGESTION_ROW_INVALID")
	}
	var jobID, kind, status, errorCode string
	var attempt int64
	if err := database.QueryRowContext(t.Context(), "SELECT id, kind, status, attempt, COALESCE(error_code, '') FROM jobs").Scan(&jobID, &kind, &status, &attempt, &errorCode); err != nil ||
		jobID != identity.tertiary || kind != "INGEST_DOCUMENT" || status != "succeeded" || attempt < 1 || attempt > 2 || errorCode != "" {
		t.Fatal("REL001_KILL_UPLOAD_JOB_ROW_INVALID")
	}
	var chunkDocument, chunkRevision, content string
	var ordinal int64
	if err := database.QueryRowContext(t.Context(), "SELECT document_id, revision_id, ordinal, content FROM chunks").Scan(&chunkDocument, &chunkRevision, &ordinal, &content); err != nil ||
		chunkDocument != identity.primary || chunkRevision != identity.secondary || ordinal != 0 || content != template.body {
		t.Fatal("REL001_KILL_UPLOAD_CHUNK_ROW_INVALID")
	}
	var ftsDocument, ftsRevision string
	var ftsOrdinal int64
	if err := database.QueryRowContext(t.Context(), `
		SELECT c.document_id, c.revision_id, c.ordinal
		FROM chunks_fts AS f JOIN chunks AS c ON c.row_id = f.rowid
	`).Scan(&ftsDocument, &ftsRevision, &ftsOrdinal); err != nil ||
		ftsDocument != identity.primary || ftsRevision != identity.secondary || ftsOrdinal != 0 {
		t.Fatal("REL001_KILL_UPLOAD_FTS_ROW_INVALID")
	}
}

func assertREL001AskRows(t *testing.T, database *sql.DB, frame rel001KillFrame, template rel001KillTemplate, identity rel001KillIdentity, conversationID, providerEndpoint string) {
	t.Helper()
	var endpoint, model string
	var configVersion, timeout, active int64
	if err := database.QueryRowContext(t.Context(), `SELECT config_version, endpoint, model, timeout_milliseconds, is_active FROM ollama_config_versions`).Scan(
		&configVersion, &endpoint, &model, &timeout, &active); err != nil || configVersion != 1 || endpoint != providerEndpoint ||
		model != "rel001-no-context" || timeout != 1000 || active != 1 {
		t.Fatal("REL001_KILL_ASK_CONFIG_ROW_INVALID")
	}
	var storedConversation, title string
	var revision int64
	if err := database.QueryRowContext(t.Context(), "SELECT id, title, revision FROM conversations").Scan(&storedConversation, &title, &revision); err != nil ||
		storedConversation != conversationID || title != fmt.Sprintf("REL001 Ask setup %04d", frame.sequence) || revision != 1 || identity.secondary != conversationID {
		t.Fatal("REL001_KILL_ASK_CONVERSATION_ROW_INVALID")
	}
	var requestConversation, requestKey, userID, answerID, scope string
	var expectedRevision int64
	if err := database.QueryRowContext(t.Context(), `SELECT conversation_id, idempotency_key, expected_revision, user_message_id, answer_message_id, COALESCE(scope_collection_id, '') FROM ask_requests`).Scan(
		&requestConversation, &requestKey, &expectedRevision, &userID, &answerID, &scope); err != nil || requestConversation != conversationID || requestKey != template.key ||
		expectedRevision != 0 || answerID != identity.primary || scope != "" {
		t.Fatal("REL001_KILL_ASK_REQUEST_ROW_INVALID")
	}
	var body struct {
		Question string `json:"question"`
	}
	if jsonErr := json.Unmarshal([]byte(template.body), &body); jsonErr != nil {
		t.Fatal("REL001_KILL_ASK_TEMPLATE_INVALID")
	}
	rows, err := database.QueryContext(t.Context(), `SELECT id, ordinal, role, status, content, COALESCE(provider_config_version, 0), COALESCE(limitation_code, ''), COALESCE(error_code, '') FROM conversation_messages ORDER BY ordinal`)
	if err != nil {
		t.Fatal("REL001_KILL_ASK_MESSAGE_QUERY_FAILED")
	}
	defer rows.Close()
	type message struct {
		id, role, status, content, limitation, code string
		ordinal, provider                           int64
	}
	messages := make([]message, 0, 2)
	for rows.Next() {
		var item message
		if err := rows.Scan(&item.id, &item.ordinal, &item.role, &item.status, &item.content, &item.provider, &item.limitation, &item.code); err != nil {
			t.Fatal("REL001_KILL_ASK_MESSAGE_SCAN_FAILED")
		}
		messages = append(messages, item)
	}
	wantAssistantContent := rag.NoContextText
	if identity.status == "failed" {
		wantAssistantContent = store.OutcomeUncertainText
	}
	if err := rows.Err(); err != nil || len(messages) != 2 ||
		messages[0].id != userID || messages[0].ordinal != 1 || messages[0].role != "user" || messages[0].status != "completed" || messages[0].content != body.Question || messages[0].provider != 0 || messages[0].limitation != "" || messages[0].code != "" ||
		messages[1].id != answerID || messages[1].ordinal != 2 || messages[1].role != "assistant" || messages[1].status != identity.status || messages[1].content != wantAssistantContent || messages[1].provider != 1 || messages[1].limitation != identity.limitation || messages[1].code != identity.code || identity.tertiary != strconv.FormatInt(revision, 10) {
		t.Fatal("REL001_KILL_ASK_MESSAGE_ROWS_INVALID")
	}
}

func assertREL001BlobTree(t *testing.T, vaultRoot string, template rel001KillTemplate, wantObjects int64) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(vaultRoot, "blobs", "staging"))
	if err != nil || len(entries) != 0 {
		t.Fatal("REL001_KILL_PRODUCT_BLOB_STAGING_NOT_EMPTY")
	}
	objectsRoot := filepath.Join(vaultRoot, "blobs", "objects", "sha256")
	if got := countBlobFiles(t, vaultRoot); got != wantObjects {
		t.Fatalf("REL001_KILL_PRODUCT_BLOB_OBJECT_COUNT_INVALID got=%d want=%d", got, wantObjects)
	}
	entries, err = os.ReadDir(objectsRoot)
	if err != nil {
		t.Fatal("REL001_KILL_PRODUCT_BLOB_OBJECT_ROOT_READ_FAILED")
	}
	if wantObjects == 0 {
		if len(entries) != 0 {
			t.Fatal("REL001_KILL_PRODUCT_BLOB_OBJECT_TREE_NOT_EMPTY")
		}
		return
	}
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(template.body)))
	if len(entries) != 1 || entries[0].Name() != digest[:2] || !entries[0].IsDir() {
		t.Fatal("REL001_KILL_PRODUCT_BLOB_PREFIX_TREE_INVALID")
	}
	prefix := filepath.Join(objectsRoot, digest[:2])
	leaves, err := os.ReadDir(prefix)
	if err != nil || len(leaves) != 1 || leaves[0].Name() != digest[2:] || leaves[0].IsDir() || leaves[0].Type()&os.ModeSymlink != 0 {
		t.Fatal("REL001_KILL_PRODUCT_BLOB_LEAF_TREE_INVALID")
	}
	leaf := filepath.Join(prefix, digest[2:])
	info, err := os.Lstat(leaf)
	if err != nil || !info.Mode().IsRegular() || info.Size() != int64(len(template.body)) {
		t.Fatal("REL001_KILL_PRODUCT_BLOB_LEAF_IDENTITY_INVALID")
	}
	content, err := os.ReadFile(leaf)
	if err != nil || string(content) != template.body || fmt.Sprintf("%x", sha256.Sum256(content)) != digest {
		t.Fatal("REL001_KILL_PRODUCT_BLOB_LEAF_CONTENT_INVALID")
	}
}
