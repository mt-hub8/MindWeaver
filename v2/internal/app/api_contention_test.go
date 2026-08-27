package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/lifecycle"
	"github.com/mt-hub8/MindWeaver/v2/internal/localhttp"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"github.com/mt-hub8/MindWeaver/v2/internal/transport"
	"github.com/mt-hub8/MindWeaver/v2/internal/workbench"
	"github.com/ncruces/go-sqlite3"
)

type contentionTestService struct {
	searchTestService
	err error
}

func (service *contentionTestService) Upload(context.Context, workbench.UploadRequest) (workbench.UploadResult, error) {
	return workbench.UploadResult{}, service.err
}

func (service *contentionTestService) ListDocumentsPage(context.Context, int, *store.DocumentCursor) (store.DocumentPage, error) {
	return store.DocumentPage{}, service.err
}

func (service *contentionTestService) ListCollectionsPage(context.Context, int, *store.CollectionCursor) (store.CollectionPage, error) {
	return store.CollectionPage{}, service.err
}

type contentionTestLifecycle struct {
	err error
}

func (*contentionTestLifecycle) TrashExpected(context.Context, string, int64) (store.DocumentLifecycle, error) {
	panic("unexpected TrashExpected call")
}

func (*contentionTestLifecycle) RestoreExpected(context.Context, string, int64) (store.DocumentLifecycle, error) {
	panic("unexpected RestoreExpected call")
}

func (*contentionTestLifecycle) PurgeExpected(context.Context, string, int64) (lifecycle.PurgeResult, error) {
	panic("unexpected PurgeExpected call")
}

func (service *contentionTestLifecycle) PurgeStatus(context.Context, string) (store.DocumentPurgeStatus, error) {
	return store.DocumentPurgeStatus{}, service.err
}

func (service *contentionTestLifecycle) ListPurges(context.Context, int, *store.DocumentPurgeCursor) (store.DocumentPurgePage, error) {
	return store.DocumentPurgePage{}, service.err
}

func TestSQLiteContentionIsAContentFreeRetryableHTTPFailure(t *testing.T) {
	const canary = "database-detail-canary-must-not-escape"
	cause := fmt.Errorf("%s: %w", canary, sqlite3.BUSY_TIMEOUT)
	service := &contentionTestService{err: cause}
	lifecycleService := &contentionTestLifecycle{err: cause}
	api := &API{service: service, lifecycle: lifecycleService}
	router := localhttp.NewRouter()
	for _, route := range []struct {
		method  string
		path    string
		handler http.HandlerFunc
	}{
		{http.MethodPost, apiPrefix + "/documents/upload", api.upload},
		{http.MethodGet, apiPrefix + "/documents", api.documents},
		{http.MethodGet, apiPrefix + "/collections", api.collections},
		{http.MethodGet, apiPrefix + "/documents/purge-status", api.purgeStatus},
		{http.MethodGet, apiPrefix + "/documents/purges", api.purges},
	} {
		if err := router.HandleFunc(route.method, route.path, route.handler); err != nil {
			t.Fatal(err)
		}
	}
	server, bootstrap, err := localhttp.Start(router, localhttp.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			t.Errorf("shutdown contention server: %v", err)
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
	var session struct {
		CSRFToken string `json:"csrfToken"`
	}
	decodeErr := json.NewDecoder(exchangeResponse.Body).Decode(&session)
	closeErr := exchangeResponse.Body.Close()
	cookies := exchangeResponse.Cookies()
	if decodeErr != nil || closeErr != nil || exchangeResponse.StatusCode != http.StatusOK ||
		session.CSRFToken == "" || len(cookies) != 1 {
		t.Fatalf("bootstrap = status:%d cookies:%d csrf:%t decode:%v close:%v",
			exchangeResponse.StatusCode, len(cookies), session.CSRFToken != "", decodeErr, closeErr)
	}

	tests := []struct {
		name    string
		method  string
		target  string
		body    string
		prepare func(*http.Request)
	}{
		{
			name: "upload write", method: http.MethodPost, target: apiPrefix + "/documents/upload", body: "bounded source",
			prepare: func(request *http.Request) {
				request.Header.Set("Content-Type", "application/octet-stream")
				request.Header.Set("Idempotency-Key", "retry-the-same-upload")
				request.Header.Set("X-MindWeaver-Title-B64", base64.RawURLEncoding.EncodeToString([]byte("title")))
				request.Header.Set("X-MindWeaver-Filename-B64", base64.RawURLEncoding.EncodeToString([]byte("source.txt")))
			},
		},
		{name: "documents read", method: http.MethodGet, target: apiPrefix + "/documents"},
		{name: "collections read", method: http.MethodGet, target: apiPrefix + "/collections"},
		{name: "purge status read", method: http.MethodGet, target: apiPrefix + "/documents/purge-status?id=document-1"},
		{name: "purges read", method: http.MethodGet, target: apiPrefix + "/documents/purges"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := http.NewRequest(test.method, server.Origin()+test.target, strings.NewReader(test.body))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Origin", server.Origin())
			request.AddCookie(cookies[0])
			if test.method != http.MethodGet && test.method != http.MethodHead {
				request.Header.Set(localhttp.CSRFHeader, session.CSRFToken)
			}
			if test.prepare != nil {
				test.prepare(request)
			}
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			body, readErr := io.ReadAll(response.Body)
			closeErr := response.Body.Close()
			if readErr != nil || closeErr != nil {
				t.Fatalf("read/close response = %v/%v", readErr, closeErr)
			}
			if response.StatusCode != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, body = %s", response.StatusCode, body)
			}
			var problem transport.Problem
			if err := json.Unmarshal(body, &problem); err != nil {
				t.Fatalf("decode Problem: %v", err)
			}
			if err := problem.Validate(); err != nil {
				t.Fatalf("invalid Problem: %v", err)
			}
			if problem.Code != transport.CodeServiceUnavailable || !problem.Retryable ||
				problem.UserAction != transport.ActionRetryLater ||
				problem.Detail != "本地数据库暂时繁忙；请稍后按原请求重试。" {
				t.Fatalf("contention Problem = %#v", problem)
			}
			if strings.Contains(string(body), canary) || strings.Contains(string(body), "sqlite3") {
				t.Fatalf("contention response leaked internal error: %s", body)
			}
		})
	}
}
