package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/mt-hub8/MindWeaver/v2/internal/localhttp"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"github.com/mt-hub8/MindWeaver/v2/internal/workbench"
)

type searchTestService struct {
	hits       []store.ChunkHit
	limits     []int
	cancelled  []string
	cancelHook func(string)
}

func (*searchTestService) Upload(context.Context, workbench.UploadRequest) (workbench.UploadResult, error) {
	panic("unexpected Upload call")
}

func (*searchTestService) GetJob(context.Context, string) (store.Job, error) {
	panic("unexpected GetJob call")
}

func (service *searchTestService) CancelJob(_ context.Context, id string) error {
	service.cancelled = append(service.cancelled, id)
	if service.cancelHook != nil {
		service.cancelHook(id)
	}
	return nil
}

func (*searchTestService) RetryDocumentIngestion(context.Context, string, int64) (store.Document, bool, error) {
	panic("unexpected RetryDocumentIngestion call")
}

func (*searchTestService) ListDocuments(context.Context, int) ([]store.Document, error) {
	panic("unexpected ListDocuments call")
}

func (*searchTestService) ListDocumentsPage(context.Context, int, *store.DocumentCursor) (store.DocumentPage, error) {
	panic("unexpected ListDocumentsPage call")
}

func (service *searchTestService) Search(_ context.Context, _ string, limit int) ([]store.ChunkHit, error) {
	service.limits = append(service.limits, limit)
	return service.prefix(limit), nil
}

func (service *searchTestService) SearchCollection(_ context.Context, _, _ string, limit int) ([]store.ChunkHit, error) {
	service.limits = append(service.limits, limit)
	return service.prefix(limit), nil
}

func (*searchTestService) CreateCollection(context.Context, string) (store.Collection, error) {
	panic("unexpected CreateCollection call")
}

func (*searchTestService) CreateCollectionIdempotent(context.Context, string, string) (store.Collection, bool, error) {
	panic("unexpected CreateCollectionIdempotent call")
}

func (*searchTestService) ListCollectionsPage(context.Context, int, *store.CollectionCursor) (store.CollectionPage, error) {
	panic("unexpected ListCollectionsPage call")
}

func (*searchTestService) ListCollectionMembersPage(context.Context, string, int, *store.CollectionMemberCursor) (store.CollectionMemberPage, error) {
	panic("unexpected ListCollectionMembersPage call")
}

func (*searchTestService) AddDocumentToCollection(context.Context, string, string) error {
	panic("unexpected AddDocumentToCollection call")
}

func (*searchTestService) AddDocumentToCollectionExpected(context.Context, string, string, int64) (store.Collection, bool, error) {
	panic("unexpected AddDocumentToCollectionExpected call")
}

func (*searchTestService) RemoveDocumentFromCollectionExpected(context.Context, string, string, int64) (store.Collection, bool, error) {
	panic("unexpected RemoveDocumentFromCollectionExpected call")
}

func (service *searchTestService) prefix(limit int) []store.ChunkHit {
	if limit > len(service.hits) {
		limit = len(service.hits)
	}
	return append([]store.ChunkHit(nil), service.hits[:limit]...)
}

func TestCancelJobPersistsBeforeInterruptingMatchingWorkerContext(t *testing.T) {
	runner := &cancellationRunner{entered: make(chan struct{}, 1), cancelled: make(chan struct{}, 1)}
	worker := newIngestionWorker(runner, nil, time.Second, time.Minute)
	worker.Start()
	t.Cleanup(func() {
		worker.Stop()
		wait, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := worker.Wait(wait); err != nil {
			t.Errorf("worker wait: %v", err)
		}
	})
	select {
	case <-runner.entered:
	case <-time.After(time.Second):
		t.Fatal("worker did not enter cancellable job")
	}
	service := &searchTestService{cancelHook: func(id string) {
		if id != "cancel-active" {
			t.Fatalf("durable cancellation id = %q", id)
		}
		runner.cancelRequested.Store(true)
	}}
	api := &API{service: service, worker: worker}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/cancel", strings.NewReader(`{"id":"cancel-active"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.cancelJob(response, request)
	if response.Code != http.StatusNoContent || len(service.cancelled) != 1 || service.cancelled[0] != "cancel-active" {
		t.Fatalf("cancel response/requests = %d/%v", response.Code, service.cancelled)
	}
	select {
	case <-runner.cancelled:
	case <-time.After(time.Second):
		t.Fatal("API cancellation did not interrupt active worker context")
	}
	if runner.cancelWithoutDurable.Load() {
		t.Fatal("worker context was cancelled before durable authority")
	}
}

func TestSearchBoundsControlHeavyJSONAndContinuesWithoutDuplicates(t *testing.T) {
	const hitCount = 100
	hits := controlHeavyHits(hitCount)
	service := &searchTestService{hits: hits}
	api := &API{service: service}

	first := performSearch(t, api, 0, 100)
	if len(first.body) > maxSearchJSON {
		t.Fatalf("first encoded page = %d bytes, budget = %d", len(first.body), maxSearchJSON)
	}
	if len(first.page.Hits) == 0 || len(first.page.Hits) >= hitCount || first.page.NextOffset == nil || *first.page.NextOffset != len(first.page.Hits) {
		t.Fatalf("first page hits/next = %d/%v", len(first.page.Hits), first.page.NextOffset)
	}
	seen := make(map[string]struct{}, hitCount)
	for _, hit := range first.page.Hits {
		assertBoundedSearchHit(t, hit)
		seen[hit.ChunkID] = struct{}{}
	}

	second := performSearch(t, api, *first.page.NextOffset, 100)
	if len(second.body) > maxSearchJSON || second.page.NextOffset != nil {
		t.Fatalf("second encoded page/next = %d/%v", len(second.body), second.page.NextOffset)
	}
	for _, hit := range second.page.Hits {
		assertBoundedSearchHit(t, hit)
		if _, duplicate := seen[hit.ChunkID]; duplicate {
			t.Fatalf("continuation repeated %q", hit.ChunkID)
		}
		seen[hit.ChunkID] = struct{}{}
	}
	if len(seen) != hitCount {
		t.Fatalf("two pages returned %d unique hits, want %d", len(seen), hitCount)
	}
	if len(service.limits) != 2 || service.limits[0] != maxSearchWindow || service.limits[1] != maxSearchWindow {
		t.Fatalf("store search limits = %v", service.limits)
	}
}

func TestSearchControlHeavyResultsStaySuccessfulThroughFourMiBBoundary(t *testing.T) {
	service := &searchTestService{hits: controlHeavyHits(100)}
	api := &API{service: service}
	router := localhttp.NewRouter()
	if err := router.HandleFunc(http.MethodGet, apiPrefix+"/search", api.search); err != nil {
		t.Fatal(err)
	}
	server, bootstrap, err := localhttp.Start(router, localhttp.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown() error = %v", err)
		}
	})
	client := &http.Client{Timeout: 5 * time.Second}
	t.Cleanup(client.CloseIdleConnections)

	exchange, err := http.NewRequest(http.MethodPost, server.Origin()+localhttp.BootstrapExchangePath, nil)
	if err != nil {
		t.Fatal(err)
	}
	exchange.Header.Set("Origin", server.Origin())
	exchange.Header.Set(localhttp.BootstrapHeader, bootstrap.HeaderValue())
	exchangeResponse, err := client.Do(exchange)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, exchangeResponse.Body)
	exchangeResponse.Body.Close()
	cookies := exchangeResponse.Cookies()
	if exchangeResponse.StatusCode != http.StatusOK || len(cookies) != 1 {
		t.Fatalf("bootstrap status/cookies = %d/%d", exchangeResponse.StatusCode, len(cookies))
	}

	request, err := http.NewRequest(http.MethodGet, server.Origin()+apiPrefix+"/search?q=abc&limit=100", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", server.Origin())
	request.AddCookie(cookies[0])
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if response.StatusCode != http.StatusOK || len(body) > maxSearchJSON {
		t.Fatalf("bounded search status/body bytes = %d/%d", response.StatusCode, len(body))
	}
	var page searchPage
	if err := json.Unmarshal(body, &page); err != nil || len(page.Hits) == 0 || page.NextOffset == nil {
		t.Fatalf("bounded search page = %#v, err=%v", page, err)
	}
}

func TestBoundedSnippetPreservesValidUTF8Boundary(t *testing.T) {
	snippet, truncated := boundedSnippet(strings.Repeat("界", maxSnippetBytes))
	if !truncated || len(snippet) > maxSnippetBytes || !utf8.ValidString(snippet) {
		t.Fatalf("snippet bytes/valid/truncated = %d/%v/%v", len(snippet), utf8.ValidString(snippet), truncated)
	}
}

type recordedSearch struct {
	body []byte
	page searchPage
}

func performSearch(t *testing.T, api *API, offset, limit int) recordedSearch {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/search?q=abc&limit=%d&offset=%d", limit, offset), nil)
	response := httptest.NewRecorder()
	api.search(response, request)
	result := response.Result()
	defer result.Body.Close()
	body := response.Body.Bytes()
	if result.StatusCode != http.StatusOK {
		t.Fatalf("search offset %d status/body = %d %q", offset, result.StatusCode, body)
	}
	var page searchPage
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("decode offset %d: %v", offset, err)
	}
	return recordedSearch{body: append([]byte(nil), body...), page: page}
}

func assertBoundedSearchHit(t *testing.T, hit searchHitView) {
	t.Helper()
	if len(hit.Snippet) > maxSnippetBytes || !utf8.ValidString(hit.Snippet) || !hit.SnippetTruncated {
		t.Fatalf("hit %q snippet bytes/valid/truncated = %d/%v/%v", hit.ChunkID, len(hit.Snippet), utf8.ValidString(hit.Snippet), hit.SnippetTruncated)
	}
}

func controlHeavyHits(count int) []store.ChunkHit {
	hits := make([]store.ChunkHit, 0, count)
	content := strings.Repeat("\x01", 64<<10)
	for index := range count {
		hits = append(hits, store.ChunkHit{
			ChunkID: fmt.Sprintf("chunk-%03d", index), DocumentID: fmt.Sprintf("document-%03d", index),
			RevisionID: fmt.Sprintf("revision-%03d", index), Ordinal: index, Content: content, Rank: float64(index),
		})
	}
	return hits
}
