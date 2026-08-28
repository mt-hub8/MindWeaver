package app

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptrace"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
)

type rel001LifecycleHTTPFixture struct {
	options  Options
	app      *App
	client   *http.Client
	session  testSession
	database string
	blobRoot string
}

type rel001LifecycleUpload struct {
	key      string
	body     []byte
	blobID   string
	identity rel001UploadIdentity
	document documentView
}

type rel001LifecycleProjection struct {
	counts             map[string]int
	collectionID       string
	collectionRevision int64
	members            []string
	documents          map[string]rel001LifecycleDocumentProjection
	uploads            []rel001LifecycleUpload
	lifecycle          map[string]int64
}

type rel001LifecycleDocumentProjection struct {
	status   string
	revision int64
}

func TestREL001ConcurrentHTTPLifecycle(t *testing.T) {
	t.Run("same collection revision elects one member", testREL001ConcurrentCollectionRevision)
	t.Run("membership add linearizes with trash", testREL001ConcurrentMembershipTrash)
}

func testREL001ConcurrentCollectionRevision(t *testing.T) {
	fixture := newREL001LifecycleHTTPFixture(t)
	first := fixture.upload(t, "rel001-lifecycle-a-first", []byte("REL001 lifecycle alpha first document phrase"))
	second := fixture.upload(t, "rel001-lifecycle-a-second", []byte("REL001 lifecycle beta second document phrase"))
	collection := fixture.createCollection(t, "rel001-lifecycle-a-collection", "REL001 lifecycle A")

	firstBody := rel001MembershipBody(t, collection.ID, first.identity.documentID, collection.Revision)
	secondBody := rel001MembershipBody(t, collection.ID, second.identity.documentID, collection.Revision)
	barrier := newREL001HTTPAdmissionBarrier(t, 2)
	results := fixture.concurrentJSON(t, barrier,
		fixture.barrierJSON(t, http.MethodPost, "/api/v1/collections/members", firstBody, barrier),
		fixture.barrierJSON(t, http.MethodPost, "/api/v1/collections/members", secondBody, barrier),
	)
	resolved := []httpResult{
		fixture.retry503(t, results[0], func() *http.Request {
			return fixture.jsonRequest(t, http.MethodPost, "/api/v1/collections/members", firstBody)
		}),
		fixture.retry503(t, results[1], func() *http.Request {
			return fixture.jsonRequest(t, http.MethodPost, "/api/v1/collections/members", secondBody)
		}),
	}

	winner := -1
	var finalCollection collectionView
	for index, result := range resolved {
		switch result.StatusCode {
		case http.StatusOK:
			got, changed := rel001DecodeMembership(t, result)
			if winner != -1 || !changed || got.ID != collection.ID || got.Revision <= collection.Revision {
				t.Fatalf("collection winner %d = %#v changed=%t, prior=%d", index, got, changed, winner)
			}
			winner, finalCollection = index, got
		case http.StatusConflict:
			rel001RequireConflictProblem(t, result)
		default:
			t.Fatalf("collection participant %d status/body = %d %q", index, result.StatusCode, result.body)
		}
	}
	if winner < 0 || resolved[1-winner].StatusCode != http.StatusConflict {
		t.Fatalf("collection election winner/statuses = %d/%d/%d", winner, resolved[0].StatusCode, resolved[1].StatusCode)
	}
	uploads := []rel001LifecycleUpload{first, second}
	winnerUpload, loserUpload := uploads[winner], uploads[1-winner]
	if members := listAllMembersHTTP(t, fixture.client, fixture.app, fixture.session, collection.ID, 10); len(members) != 1 || members[0].ID != winnerUpload.identity.documentID {
		t.Fatalf("collection members after election = %#v", members)
	}
	replay, changed := rel001DecodeMembership(t, do(t, fixture.client,
		fixture.jsonRequest(t, http.MethodPost, "/api/v1/collections/members", rel001MembershipBody(t, collection.ID, winnerUpload.identity.documentID, collection.Revision))))
	if changed || replay.ID != collection.ID || replay.Revision != finalCollection.Revision {
		t.Fatalf("winning stale replay = %#v changed=%t", replay, changed)
	}
	rel001RequireConflictProblem(t, do(t, fixture.client,
		fixture.jsonRequest(t, http.MethodPost, "/api/v1/collections/members", rel001MembershipBody(t, collection.ID, loserUpload.identity.documentID, collection.Revision))))

	fixture.restart(t, 0)
	fixture.requireCollection(t, collection.ID, finalCollection.Revision, []string{winnerUpload.identity.documentID})
	fixture.requireLive(t, []string{first.blobID, second.blobID})
	fixture.shutdown(t)
	fixture.assertRaw(t, rel001LifecycleProjection{
		counts: rel001LifecycleTableCounts(2, 1, 1, 0), collectionID: collection.ID,
		collectionRevision: finalCollection.Revision, members: []string{winnerUpload.identity.documentID},
		documents: map[string]rel001LifecycleDocumentProjection{
			first.identity.documentID:  {status: "active", revision: first.document.Revision},
			second.identity.documentID: {status: "active", revision: second.document.Revision},
		},
		uploads: uploads,
	})
	rel001AssertBlobProjection(t, fixture.blobRoot, map[string][]byte{first.blobID: first.body, second.blobID: second.body})
}

func testREL001ConcurrentMembershipTrash(t *testing.T) {
	fixture := newREL001LifecycleHTTPFixture(t)
	upload := fixture.upload(t, "rel001-lifecycle-b-upload", []byte("REL001 lifecycle trash collision searchable phrase"))
	collection := fixture.createCollection(t, "rel001-lifecycle-b-collection", "REL001 lifecycle B")
	addBody := rel001MembershipBody(t, collection.ID, upload.identity.documentID, collection.Revision)
	trashBody := rel001DocumentMutationBody(t, upload.identity.documentID, upload.document.Revision)
	barrier := newREL001HTTPAdmissionBarrier(t, 2)
	results := fixture.concurrentJSON(t, barrier,
		fixture.barrierJSON(t, http.MethodPost, "/api/v1/collections/members", addBody, barrier),
		fixture.barrierJSON(t, http.MethodPost, "/api/v1/documents/trash", trashBody, barrier),
	)
	addResult := fixture.retry503(t, results[0], func() *http.Request {
		return fixture.jsonRequest(t, http.MethodPost, "/api/v1/collections/members", addBody)
	})
	trashResult := fixture.retry503(t, results[1], func() *http.Request {
		return fixture.jsonRequest(t, http.MethodPost, "/api/v1/documents/trash", trashBody)
	})
	trashed := rel001DecodeLifecycle(t, trashResult)
	if trashResult.StatusCode != http.StatusOK || trashed.DocumentID != upload.identity.documentID || trashed.Status != "trashed" ||
		trashed.Revision <= upload.document.Revision || trashed.TrashedAt == nil || trashed.PurgeRequestedAt != nil {
		t.Fatalf("trash result = %d %#v", trashResult.StatusCode, trashed)
	}
	trashedAt, err := time.Parse(time.RFC3339Nano, *trashed.TrashedAt)
	if err != nil || trashedAt.UnixMicro() != trashed.Revision {
		t.Fatalf("trash timestamp/revision = %q/%d, error=%v", *trashed.TrashedAt, trashed.Revision, err)
	}

	membershipCount := 0
	finalRevision := collection.Revision
	switch addResult.StatusCode {
	case http.StatusOK:
		updated, changed := rel001DecodeMembership(t, addResult)
		if !changed || updated.ID != collection.ID || updated.Revision <= collection.Revision {
			t.Fatalf("add-before-trash result = %#v changed=%t", updated, changed)
		}
		membershipCount, finalRevision = 1, updated.Revision
	case http.StatusConflict:
		rel001RequireConflictProblem(t, addResult)
	default:
		t.Fatalf("add-vs-trash status/body = %d %q", addResult.StatusCode, addResult.body)
	}
	fixture.requireCollection(t, collection.ID, finalRevision, rel001OptionalMember(membershipCount, upload.identity.documentID))
	fixture.requireSearch(t, "searchable phrase", "", nil)
	fixture.requireSearch(t, "searchable phrase", collection.ID, nil)
	rel001RequireConflictProblem(t, do(t, fixture.client,
		fixture.jsonRequest(t, http.MethodPost, "/api/v1/collections/members", addBody)))

	fixture.restart(t, 0)
	fixture.requireCollection(t, collection.ID, finalRevision, rel001OptionalMember(membershipCount, upload.identity.documentID))
	fixture.requireSearch(t, "searchable phrase", "", nil)
	fixture.requireSearch(t, "searchable phrase", collection.ID, nil)
	fixture.requireLive(t, []string{upload.blobID})
	fixture.shutdown(t)
	fixture.assertRaw(t, rel001LifecycleProjection{
		counts: rel001LifecycleTableCounts(1, 1, membershipCount, 1), collectionID: collection.ID,
		collectionRevision: finalRevision, members: rel001OptionalMember(membershipCount, upload.identity.documentID),
		documents: map[string]rel001LifecycleDocumentProjection{
			upload.identity.documentID: {status: "trashed", revision: trashed.Revision},
		},
		uploads: []rel001LifecycleUpload{upload}, lifecycle: map[string]int64{upload.identity.documentID: trashed.Revision},
	})
	rel001AssertBlobProjection(t, fixture.blobRoot, map[string][]byte{upload.blobID: upload.body})
}

func newREL001LifecycleHTTPFixture(t *testing.T) *rel001LifecycleHTTPFixture {
	t.Helper()
	options := rel001ConcurrentOptions(t.TempDir())
	application := startTestApp(t, options)
	paths := application.vault.Paths()
	client := rel001ConcurrentHTTPClient(t)
	return &rel001LifecycleHTTPFixture{
		options: options, app: application, client: client, session: exchangeApp(t, client, application),
		database: filepath.Join(paths.Data, store.DatabaseFileName), blobRoot: paths.Blobs,
	}
}

func (fixture *rel001LifecycleHTTPFixture) shutdown(t *testing.T) {
	t.Helper()
	shutdownTestApp(t, fixture.app)
}

func (fixture *rel001LifecycleHTTPFixture) restart(t *testing.T, swept int) {
	t.Helper()
	fixture.shutdown(t)
	fixture.app = startTestApp(t, fixture.options)
	rel001RequireRestartEvidence(t, fixture.app, swept)
	fixture.session = exchangeApp(t, fixture.client, fixture.app)
}

func (fixture *rel001LifecycleHTTPFixture) upload(t *testing.T, key string, body []byte) rel001LifecycleUpload {
	t.Helper()
	result := do(t, fixture.client, rel001UploadRequest(t, fixture.app, fixture.session, key, body))
	if result.StatusCode != http.StatusAccepted {
		t.Fatalf("setup upload status/body = %d %q", result.StatusCode, result.body)
	}
	wire := rel001DecodeUpload(t, result)
	if !wire.Created {
		t.Fatal("setup upload did not create a graph")
	}
	identity := rel001UploadIdentity{wire.DocumentID, wire.RevisionID, wire.JobID}
	job := waitForJob(t, fixture.client, fixture.app, fixture.session, identity.jobID, "succeeded")
	if job.Attempt != 1 || job.ErrorCode != "" {
		t.Fatalf("setup ingestion job = %#v", job)
	}
	document := requireDocumentView(t, listAllDocumentsHTTP(t, fixture.client, fixture.app, fixture.session, 20), identity.documentID)
	if document.Status != "active" || document.ActiveRevisionID != identity.revisionID || document.IngestionJobID != identity.jobID || document.IngestionStatus != "succeeded" {
		t.Fatalf("setup document = %#v", document)
	}
	return rel001LifecycleUpload{key: key, body: slices.Clone(body), blobID: rel001BlobID(body), identity: identity, document: document}
}

func (fixture *rel001LifecycleHTTPFixture) createCollection(t *testing.T, key, name string) collectionView {
	t.Helper()
	body, err := json.Marshal(struct {
		Name string `json:"name"`
	}{name})
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.jsonRequest(t, http.MethodPost, "/api/v1/collections", body)
	request.Header.Set("Idempotency-Key", key)
	result := do(t, fixture.client, request)
	var payload struct {
		Collection collectionView `json:"collection"`
		Created    *bool          `json:"created"`
	}
	if result.StatusCode != http.StatusCreated || json.Unmarshal(result.body, &payload) != nil || payload.Created == nil || !*payload.Created || payload.Collection.ID == "" || payload.Collection.Revision <= 0 {
		t.Fatalf("create collection status/payload = %d %#v", result.StatusCode, payload)
	}
	return payload.Collection
}

func (fixture *rel001LifecycleHTTPFixture) jsonRequest(t *testing.T, method, path string, body []byte) *http.Request {
	t.Helper()
	request := appRequest(t, fixture.app, fixture.session, method, path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	return request
}

func (fixture *rel001LifecycleHTTPFixture) barrierJSON(t *testing.T, method, path string, body []byte, barrier *rel001HTTPAdmissionBarrier) *http.Request {
	t.Helper()
	request := appRequest(t, fixture.app, fixture.session, method, path,
		&rel001BarrierBody{barrier: barrier, body: slices.Clone(body)})
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Expect", "100-continue")
	var once sync.Once
	trace := &httptrace.ClientTrace{Got100Continue: func() {
		once.Do(func() { barrier.arrive(&barrier.remainingHandlers, barrier.handlersReady) })
	}}
	return request.WithContext(httptrace.WithClientTrace(request.Context(), trace))
}

func (fixture *rel001LifecycleHTTPFixture) concurrentJSON(t *testing.T, barrier *rel001HTTPAdmissionBarrier, requests ...*http.Request) []rel001HTTPResult {
	t.Helper()
	if len(requests) != 2 {
		t.Fatalf("lifecycle HTTP participants = %d", len(requests))
	}
	wave := rel001StartHTTPWave(fixture.client, requests)
	barrier.awaitAndRelease(t)
	collected := rel001CollectHTTPResults(t, wave, len(requests))
	indexed := make([]rel001HTTPResult, len(requests))
	seen := make([]bool, len(requests))
	for _, result := range collected {
		if result.index < 0 || result.index >= len(requests) || seen[result.index] {
			t.Fatalf("lifecycle HTTP result index = %d", result.index)
		}
		seen[result.index] = true
		indexed[result.index] = result
	}
	return indexed
}

func (fixture *rel001LifecycleHTTPFixture) retry503(t *testing.T, initial rel001HTTPResult, request func() *http.Request) httpResult {
	t.Helper()
	rel001RequireHTTPResult(t, initial)
	result := initial.httpResult
	for attempt := 0; result.StatusCode == http.StatusServiceUnavailable && attempt < 3; attempt++ {
		rel001RequireRetryableProblem(t, result)
		result = do(t, fixture.client, request())
	}
	if result.StatusCode == http.StatusServiceUnavailable {
		rel001RequireRetryableProblem(t, result)
		t.Fatalf("SERVICE_UNAVAILABLE did not converge after bounded exact retries")
	}
	return result
}

func rel001MembershipBody(t *testing.T, collectionID, documentID string, expectedRevision int64) []byte {
	t.Helper()
	body, err := json.Marshal(struct {
		CollectionID     string `json:"collectionId"`
		DocumentID       string `json:"documentId"`
		ExpectedRevision int64  `json:"expectedRevision"`
	}{collectionID, documentID, expectedRevision})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func rel001DocumentMutationBody(t *testing.T, documentID string, expectedRevision int64) []byte {
	t.Helper()
	body, err := json.Marshal(documentMutationInput{DocumentID: documentID, ExpectedRevision: expectedRevision})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func rel001DecodeMembership(t *testing.T, result httpResult) (collectionView, bool) {
	t.Helper()
	var payload struct {
		Collection collectionView `json:"collection"`
		Changed    *bool          `json:"changed"`
	}
	if result.StatusCode != http.StatusOK || json.Unmarshal(result.body, &payload) != nil || payload.Changed == nil || payload.Collection.ID == "" || payload.Collection.Revision <= 0 {
		t.Fatalf("membership status/payload = %d %#v", result.StatusCode, payload)
	}
	return payload.Collection, *payload.Changed
}

func rel001DecodeLifecycle(t *testing.T, result httpResult) lifecycleView {
	t.Helper()
	var payload struct {
		Document lifecycleView `json:"document"`
	}
	if result.StatusCode != http.StatusOK || json.Unmarshal(result.body, &payload) != nil || payload.Document.DocumentID == "" || payload.Document.Revision <= 0 {
		t.Fatalf("lifecycle status/payload = %d %#v", result.StatusCode, payload)
	}
	return payload.Document
}

func rel001OptionalMember(count int, documentID string) []string {
	if count == 0 {
		return nil
	}
	return []string{documentID}
}

func (fixture *rel001LifecycleHTTPFixture) requireCollection(t *testing.T, collectionID string, revision int64, wantMembers []string) {
	t.Helper()
	collections := listAllCollectionsHTTP(t, fixture.client, fixture.app, fixture.session, 10)
	if len(collections) != 1 || collections[0].ID != collectionID || collections[0].Revision != revision {
		t.Fatalf("collection catalog = %#v, want %q/%d", collections, collectionID, revision)
	}
	members := listAllMembersHTTP(t, fixture.client, fixture.app, fixture.session, collectionID, 10)
	got := make([]string, len(members))
	for index, member := range members {
		got[index] = member.ID
	}
	slices.Sort(got)
	wantMembers = slices.Clone(wantMembers)
	slices.Sort(wantMembers)
	if !slices.Equal(got, wantMembers) {
		t.Fatalf("collection members = %q, want %q", got, wantMembers)
	}
}

func (fixture *rel001LifecycleHTTPFixture) requireSearch(t *testing.T, query, collectionID string, wantDocumentIDs []string) {
	t.Helper()
	result := searchAPI(t, fixture.client, fixture.app, fixture.session, query, collectionID)
	var page searchPage
	if result.StatusCode != http.StatusOK || json.Unmarshal(result.body, &page) != nil {
		t.Fatalf("search status/body = %d %q", result.StatusCode, result.body)
	}
	got := make([]string, len(page.Hits))
	for index, hit := range page.Hits {
		got[index] = hit.DocumentID
	}
	slices.Sort(got)
	wantDocumentIDs = slices.Clone(wantDocumentIDs)
	slices.Sort(wantDocumentIDs)
	if !slices.Equal(got, wantDocumentIDs) {
		t.Fatalf("search documents = %q, want %q", got, wantDocumentIDs)
	}
}

func (fixture *rel001LifecycleHTTPFixture) requireLive(t *testing.T, blobIDs []string) {
	t.Helper()
	rel001RequireLiveConsistency(t, fixture.app, blobIDs, nil)
}

func rel001LifecycleTableCounts(documents, collections, memberships, lifecycle int) map[string]int {
	counts := rel001UploadTableCounts(0)
	for _, table := range []string{"documents", "document_revisions", "document_ingestions", "jobs", "chunks", "chunks_fts"} {
		counts[table] = documents
	}
	counts["collections"] = collections
	counts["collection_documents"] = memberships
	counts["document_lifecycle"] = lifecycle
	return counts
}

func (fixture *rel001LifecycleHTTPFixture) assertRaw(t *testing.T, want rel001LifecycleProjection) {
	t.Helper()
	database := rel001OpenReadOnlyDatabase(t, fixture.database)
	defer func() {
		if err := database.Close(); err != nil {
			t.Errorf("close lifecycle projection database: %v", err)
		}
	}()
	tx, err := database.BeginTx(t.Context(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("begin lifecycle projection: %v", err)
	}
	defer tx.Rollback()
	for table, expected := range want.counts {
		var count int
		if err := tx.QueryRowContext(t.Context(), "SELECT count(*) FROM "+table).Scan(&count); err != nil || count != expected {
			t.Fatalf("%s rows = %d, want %d, error=%v", table, count, expected, err)
		}
	}
	if want.collectionID != "" {
		var revision int64
		if err := tx.QueryRowContext(t.Context(), "SELECT updated_at FROM collections WHERE id = ?", want.collectionID).Scan(&revision); err != nil || revision != want.collectionRevision {
			t.Fatalf("raw collection revision = %d, want %d, error=%v", revision, want.collectionRevision, err)
		}
	}
	rel001AssertQueryStrings(t, tx, "SELECT document_id FROM collection_documents ORDER BY document_id", nil, want.members)

	rows, err := tx.QueryContext(t.Context(), "SELECT id, status, updated_at FROM documents ORDER BY id")
	if err != nil {
		t.Fatalf("read raw documents: %v", err)
	}
	gotDocuments := map[string]rel001LifecycleDocumentProjection{}
	for rows.Next() {
		var id, status string
		var revision int64
		if err := rows.Scan(&id, &status, &revision); err != nil {
			_ = rows.Close()
			t.Fatalf("scan raw document: %v", err)
		}
		gotDocuments[id] = rel001LifecycleDocumentProjection{status: status, revision: revision}
	}
	if err := rel001FinishRows(rows); err != nil {
		t.Fatalf("close raw documents: %v", err)
	}
	if len(gotDocuments) != len(want.documents) {
		t.Fatalf("documents size = %d, want %d", len(gotDocuments), len(want.documents))
	}
	for id, expected := range want.documents {
		if gotDocuments[id] != expected {
			t.Fatalf("document %q = %#v, want %#v", id, gotDocuments[id], expected)
		}
	}

	rows, err = tx.QueryContext(t.Context(), `SELECT document_id, trashed_at, purge_requested_at FROM document_lifecycle ORDER BY document_id`)
	if err != nil {
		t.Fatalf("read raw lifecycle: %v", err)
	}
	gotLifecycle := map[string]int64{}
	for rows.Next() {
		var id string
		var trashedAt int64
		var purgeRequestedAt sql.NullInt64
		if err := rows.Scan(&id, &trashedAt, &purgeRequestedAt); err != nil {
			_ = rows.Close()
			t.Fatalf("scan raw lifecycle: %v", err)
		}
		if purgeRequestedAt.Valid {
			t.Fatalf("raw lifecycle %q has purge request", id)
		}
		gotLifecycle[id] = trashedAt
	}
	if err := rel001FinishRows(rows); err != nil {
		t.Fatalf("close raw lifecycle: %v", err)
	}
	if len(gotLifecycle) != len(want.lifecycle) {
		t.Fatalf("lifecycle size = %d, want %d", len(gotLifecycle), len(want.lifecycle))
	}
	for id, trashedAt := range want.lifecycle {
		if gotLifecycle[id] != trashedAt {
			t.Fatalf("lifecycle %q trashed_at = %d, want %d", id, gotLifecycle[id], trashedAt)
		}
	}

	wantByKey := make(map[string]rel001LifecycleUpload, len(want.uploads))
	wantJobs := make(map[string]string, len(want.uploads))
	wantBlobs := make([]string, 0, len(want.uploads))
	for _, upload := range want.uploads {
		wantByKey[upload.key] = upload
		wantJobs[upload.identity.jobID] = "succeeded/1"
		wantBlobs = append(wantBlobs, upload.blobID)
	}
	rows, err = tx.QueryContext(t.Context(), `SELECT idempotency_key, document_id, revision_id, job_id, source_blob_id FROM document_ingestions ORDER BY idempotency_key`)
	if err != nil {
		t.Fatalf("read raw ingestions: %v", err)
	}
	seenKeys := map[string]bool{}
	for rows.Next() {
		var key, documentID, revisionID, jobID, blobID string
		if err := rows.Scan(&key, &documentID, &revisionID, &jobID, &blobID); err != nil {
			_ = rows.Close()
			t.Fatalf("scan raw ingestion: %v", err)
		}
		upload, ok := wantByKey[key]
		if !ok || seenKeys[key] || upload.identity != (rel001UploadIdentity{documentID, revisionID, jobID}) || upload.blobID != blobID {
			t.Fatalf("raw ingestion %q = %q/%q/%q/%q", key, documentID, revisionID, jobID, blobID)
		}
		seenKeys[key] = true
	}
	if err := rel001FinishRows(rows); err != nil || len(seenKeys) != len(wantByKey) {
		t.Fatalf("raw ingestion keys = %d/%d, close=%v", len(seenKeys), len(wantByKey), err)
	}

	rows, err = tx.QueryContext(t.Context(), "SELECT id, status, attempt FROM jobs ORDER BY id")
	if err != nil {
		t.Fatalf("read raw jobs: %v", err)
	}
	gotJobs := map[string]string{}
	for rows.Next() {
		var id, status string
		var attempt int
		if err := rows.Scan(&id, &status, &attempt); err != nil {
			_ = rows.Close()
			t.Fatalf("scan raw job: %v", err)
		}
		gotJobs[id] = fmt.Sprintf("%s/%d", status, attempt)
	}
	if err := rel001FinishRows(rows); err != nil {
		t.Fatalf("close raw jobs: %v", err)
	}
	rel001AssertStringMap(t, "jobs", gotJobs, wantJobs)
	rel001AssertChunkProjection(t, tx, "chunks", `SELECT document_id, revision_id, content FROM chunks ORDER BY document_id`, want.uploads)
	rel001AssertChunkProjection(t, tx, "chunks_fts", `SELECT c.document_id, c.revision_id, c.content FROM chunks_fts AS f JOIN chunks AS c ON c.row_id = f.rowid ORDER BY c.document_id`, want.uploads)
	slices.Sort(wantBlobs)
	rel001AssertQueryStrings(t, tx, `SELECT DISTINCT source_blob_id FROM document_revisions WHERE source_blob_id IS NOT NULL ORDER BY source_blob_id`, nil, wantBlobs)
	if err := tx.Commit(); err != nil {
		t.Fatalf("finish lifecycle projection: %v", err)
	}
}

func rel001AssertQueryStrings(t *testing.T, tx *sql.Tx, query string, args []any, want []string) {
	t.Helper()
	rows, err := tx.QueryContext(t.Context(), query, args...)
	if err != nil {
		t.Fatalf("query exact strings: %v", err)
	}
	var got []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			_ = rows.Close()
			t.Fatalf("scan exact string: %v", err)
		}
		got = append(got, value)
	}
	if err := rel001FinishRows(rows); err != nil {
		t.Fatalf("close exact strings: %v", err)
	}
	want = slices.Clone(want)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("query %q = %q, want %q", query, got, want)
	}
}

func rel001AssertChunkProjection(t *testing.T, tx *sql.Tx, label, query string, uploads []rel001LifecycleUpload) {
	t.Helper()
	want := make(map[string]rel001LifecycleUpload, len(uploads))
	for _, upload := range uploads {
		want[upload.identity.documentID] = upload
	}
	rows, err := tx.QueryContext(t.Context(), query)
	if err != nil {
		t.Fatalf("read %s projection: %v", label, err)
	}
	seen := map[string]bool{}
	for rows.Next() {
		var documentID, revisionID, content string
		if err := rows.Scan(&documentID, &revisionID, &content); err != nil {
			_ = rows.Close()
			t.Fatalf("scan %s projection: %v", label, err)
		}
		upload, ok := want[documentID]
		if !ok || seen[documentID] || revisionID != upload.identity.revisionID || !bytes.Equal([]byte(content), upload.body) {
			t.Fatalf("%s projection = %q/%q/%q", label, documentID, revisionID, content)
		}
		seen[documentID] = true
	}
	if err := rel001FinishRows(rows); err != nil || len(seen) != len(want) {
		t.Fatalf("%s projection count = %d/%d, close=%v", label, len(seen), len(want), err)
	}
}

func rel001FinishRows(rows *sql.Rows) error {
	return errors.Join(rows.Err(), rows.Close())
}

func rel001AssertStringMap(t *testing.T, label string, got, want map[string]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s size = %d, want %d", label, len(got), len(want))
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("%s[%q] = %q, want %q", label, key, got[key], value)
		}
	}
}
