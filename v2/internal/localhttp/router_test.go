package localhttp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/transport"
)

func TestRouterAcceptsOnlyFixedBusinessRoutesAndSealsAtStart(t *testing.T) {
	router := NewRouter()
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	asset := []byte("immutable")
	if err := router.HandlePublicAsset("/assets/app-a1.js", "text/javascript; charset=utf-8", asset); err != nil {
		t.Fatal(err)
	}
	asset[0] = 'X'
	for _, candidate := range []struct {
		path        string
		contentType string
		body        []byte
	}{
		{path: "/api/public", contentType: "text/javascript", body: []byte("x")},
		{path: "/assets/wild*card.js", contentType: "text/javascript", body: []byte("x")},
		{path: "/assets/data.json", contentType: "application/json", body: []byte("x")},
		{path: "/assets/empty.js", contentType: "text/javascript", body: nil},
	} {
		if err := router.HandlePublicAsset(candidate.path, candidate.contentType, candidate.body); !errors.Is(err, ErrInvalidRoute) {
			t.Errorf("HandlePublicAsset(%q) error = %v", candidate.path, err)
		}
	}
	for _, candidate := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/"},
		{method: http.MethodGet, path: HealthPath},
		{method: http.MethodGet, path: "/api/"},
		{method: http.MethodGet, path: "/api/items/"},
		{method: http.MethodGet, path: "/api/{id}"},
		{method: http.MethodGet, path: "/api/%69tems"},
		{method: http.MethodOptions, path: "/api/items"},
	} {
		if err := router.Handle(candidate.method, candidate.path, handler); !errors.Is(err, ErrInvalidRoute) {
			t.Errorf("Handle(%q, %q) error = %v", candidate.method, candidate.path, err)
		}
	}
	if err := router.Handle(http.MethodGet, "/api/items", handler); err != nil {
		t.Fatal(err)
	}
	if err := router.Handle(http.MethodGet, "/api/items", handler); !errors.Is(err, ErrDuplicateRoute) {
		t.Fatalf("duplicate error = %v", err)
	}
	active := startTestServer(t, router, Config{})
	if err := router.Handle(http.MethodPost, "/api/items", handler); !errors.Is(err, ErrRouterSealed) {
		t.Fatalf("post-Start registration error = %v", err)
	}
	if err := router.HandlePublicAsset("/assets/late.js", "text/javascript", []byte("x")); !errors.Is(err, ErrRouterSealed) {
		t.Fatalf("post-Start public registration error = %v", err)
	}
	request := active.request(t, http.MethodGet, "/assets/app-a1.js", nil)
	request.Header.Del("Origin")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set("Sec-Fetch-Mode", "no-cors")
	request.Header.Set("Sec-Fetch-Dest", "script")
	response, err := active.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if string(body) != "immutable" {
		t.Fatalf("public asset was not copied: %q", body)
	}
	if active.server == nil {
		t.Fatal("server was not started")
	}
}

func TestExactPublicResourcesAndSameOriginBrowserFetch(t *testing.T) {
	router := NewRouter()
	if err := router.HandlePublicAsset("/", "text/html; charset=utf-8", []byte("safe shell")); err != nil {
		t.Fatal(err)
	}
	if err := router.HandlePublicAsset("/assets/app-deadbeef.js", "text/javascript; charset=utf-8", []byte("safe asset")); err != nil {
		t.Fatal(err)
	}
	if err := router.HandleFunc(http.MethodGet, "/api/value", func(response http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(response, "ok")
	}); err != nil {
		t.Fatal(err)
	}
	active := startTestServer(t, router, Config{})

	index := active.request(t, http.MethodGet, "/", nil)
	index.Header.Del("Origin")
	index.Header.Set("Sec-Fetch-Site", "none")
	index.Header.Set("Sec-Fetch-Mode", "navigate")
	index.Header.Set("Sec-Fetch-Dest", "document")
	indexResponse, err := active.client.Do(index)
	if err != nil {
		t.Fatal(err)
	}
	indexBody, _ := io.ReadAll(indexResponse.Body)
	indexResponse.Body.Close()
	if indexResponse.StatusCode != http.StatusOK || string(indexBody) != "safe shell" {
		t.Fatalf("index = %d %q", indexResponse.StatusCode, indexBody)
	}

	asset := active.request(t, http.MethodGet, "/assets/app-deadbeef.js", nil)
	asset.Header.Del("Origin")
	asset.Header.Set("Sec-Fetch-Site", "same-origin")
	asset.Header.Set("Sec-Fetch-Mode", "no-cors")
	asset.Header.Set("Sec-Fetch-Dest", "script")
	assetResponse, err := active.client.Do(asset)
	if err != nil {
		t.Fatal(err)
	}
	assetResponse.Body.Close()
	if assetResponse.StatusCode != http.StatusOK {
		t.Fatalf("asset status = %d", assetResponse.StatusCode)
	}

	session := active.exchange(t)
	read := active.request(t, http.MethodGet, "/api/value", nil)
	read.Header.Del("Origin")
	read.Header.Set("Sec-Fetch-Site", "same-origin")
	read.Header.Set("Sec-Fetch-Mode", "cors")
	read.Header.Set("Sec-Fetch-Dest", "empty")
	read.AddCookie(session.cookie)
	readResponse, err := active.client.Do(read)
	if err != nil {
		t.Fatal(err)
	}
	readResponse.Body.Close()
	if readResponse.StatusCode != http.StatusOK {
		t.Fatalf("same-origin fetch status = %d", readResponse.StatusCode)
	}

	badAsset := active.request(t, http.MethodGet, "/assets/app-deadbeef.js", nil)
	badAsset.Header.Del("Origin")
	badAsset.Header.Set("Sec-Fetch-Site", "cross-site")
	badAsset.Header.Set("Sec-Fetch-Mode", "no-cors")
	badAsset.Header.Set("Sec-Fetch-Dest", "script")
	badResponse, err := active.client.Do(badAsset)
	if err != nil {
		t.Fatal(err)
	}
	badResponse.Body.Close()
	if badResponse.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-site asset status = %d", badResponse.StatusCode)
	}
}

func TestOnlyValidatedProblemCanCrossSanitizingBoundary(t *testing.T) {
	router := NewRouter()
	if err := router.HandleFunc(http.MethodGet, "/api/problem", func(response http.ResponseWriter, _ *http.Request) {
		WriteProblem(response, transport.NewProblem(transport.CodeConflict, "safe conflict", "request-1"))
	}); err != nil {
		t.Fatal(err)
	}
	if err := router.HandleFunc(http.MethodGet, "/api/invalid-problem", func(response http.ResponseWriter, _ *http.Request) {
		WriteProblem(response, transport.Problem{Status: 418, Detail: "secret"})
	}); err != nil {
		t.Fatal(err)
	}
	active := startTestServer(t, router, Config{})
	session := active.exchange(t)
	for _, test := range []struct {
		path       string
		wantStatus int
		wantDetail string
	}{
		{path: "/api/problem", wantStatus: http.StatusConflict, wantDetail: "safe conflict"},
		{path: "/api/invalid-problem", wantStatus: http.StatusInternalServerError},
	} {
		request := active.request(t, http.MethodGet, test.path, nil)
		request.AddCookie(session.cookie)
		response, err := active.client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != test.wantStatus {
			t.Fatalf("%s status/body = %d %q", test.path, response.StatusCode, body)
		}
		if test.wantDetail != "" {
			var problem transport.Problem
			if err := json.Unmarshal(body, &problem); err != nil || problem.Detail != test.wantDetail {
				t.Fatalf("problem = %#v, err=%v", problem, err)
			}
		} else if strings.Contains(string(body), "secret") {
			t.Fatalf("invalid problem leaked: %q", body)
		}
	}
}

func TestBootstrapExpires(t *testing.T) {
	active := startTestServer(t, nil, Config{BootstrapTTL: time.Millisecond})
	time.Sleep(10 * time.Millisecond)
	request := active.request(t, http.MethodPost, BootstrapExchangePath, nil)
	request.Header.Set(BootstrapHeader, active.bootstrap.HeaderValue())
	response, err := active.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expired bootstrap status = %d", response.StatusCode)
	}
}
