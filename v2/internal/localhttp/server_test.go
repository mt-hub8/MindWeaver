package localhttp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type activeTestServer struct {
	server    *Server
	bootstrap BootstrapToken
	client    *http.Client
}

type browserSession struct {
	cookie *http.Cookie
	csrf   string
}

func startTestServer(t *testing.T, router *Router, config Config) activeTestServer {
	t.Helper()
	server, bootstrap, err := Start(router, config)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	client := &http.Client{
		Timeout: 2 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	t.Cleanup(func() {
		client.CloseIdleConnections()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown() error = %v", err)
		}
	})
	return activeTestServer{
		server:    server,
		bootstrap: bootstrap,
		client:    client,
	}
}

func (active activeTestServer) request(t *testing.T, method, path string, body io.Reader) *http.Request {
	t.Helper()
	request, err := http.NewRequest(method, active.server.Origin()+path, body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", active.server.Origin())
	return request
}

func (active activeTestServer) exchange(t *testing.T) browserSession {
	t.Helper()
	request := active.request(t, http.MethodPost, BootstrapExchangePath, nil)
	request.Header.Set(BootstrapHeader, active.bootstrap.HeaderValue())
	response, err := active.client.Do(request)
	if err != nil {
		t.Fatalf("bootstrap exchange: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("bootstrap status = %d, body = %q", response.StatusCode, body)
	}
	var payload struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	cookies := response.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("bootstrap cookies = %d, want 1", len(cookies))
	}
	return browserSession{cookie: cookies[0], csrf: payload.CSRFToken}
}

func TestStartBindsOnlyRandomIPv4LoopbackAndRedactsBootstrap(t *testing.T) {
	first := startTestServer(t, nil, Config{})
	second := startTestServer(t, nil, Config{})

	address, ok := first.server.Addr().(*net.TCPAddr)
	if !ok || address.IP.String() != "127.0.0.1" || address.Port == 0 {
		t.Fatalf("Addr() = %#v", first.server.Addr())
	}
	if first.server.Origin() != "http://"+first.server.Addr().String() {
		t.Fatalf("Origin() = %q", first.server.Origin())
	}
	if first.server.Addr().String() == second.server.Addr().String() {
		t.Fatalf("two servers unexpectedly used the same random address %q", first.server.Addr())
	}
	if first.bootstrap.HeaderValue() == second.bootstrap.HeaderValue() || len(first.bootstrap.HeaderValue()) != 43 {
		t.Fatal("bootstrap tokens were not independent 256-bit values")
	}
	if fmt.Sprint(first.bootstrap) != "[REDACTED]" || strings.Contains(fmt.Sprintf("%v", first.bootstrap), first.bootstrap.HeaderValue()) || strings.Contains(fmt.Sprintf("%#v", first.bootstrap), first.bootstrap.HeaderValue()) {
		t.Fatal("BootstrapToken formatting exposed its credential")
	}
}

func TestHealthRequiresExactHostOriginOrExplicitNonBrowserRead(t *testing.T) {
	active := startTestServer(t, nil, Config{})

	tests := []struct {
		name       string
		mutate     func(*http.Request)
		wantStatus int
	}{
		{name: "same origin", wantStatus: http.StatusOK},
		{name: "bad host", mutate: func(request *http.Request) { request.Host = "localhost:1" }, wantStatus: http.StatusForbidden},
		{name: "bad origin", mutate: func(request *http.Request) { request.Header.Set("Origin", "http://attacker.invalid") }, wantStatus: http.StatusForbidden},
		{name: "null origin", mutate: func(request *http.Request) { request.Header.Set("Origin", "null") }, wantStatus: http.StatusForbidden},
		{name: "missing origin", mutate: func(request *http.Request) { request.Header.Del("Origin") }, wantStatus: http.StatusForbidden},
		{name: "explicit native readonly", mutate: func(request *http.Request) {
			request.Header.Del("Origin")
			request.Header.Set(NonBrowserHeader, NonBrowserReadOnly)
		}, wantStatus: http.StatusOK},
		{name: "top-level browser navigation", mutate: func(request *http.Request) {
			request.Header.Del("Origin")
			request.Header.Set("Sec-Fetch-Site", "none")
			request.Header.Set("Sec-Fetch-Mode", "navigate")
			request.Header.Set("Sec-Fetch-Dest", "document")
		}, wantStatus: http.StatusOK},
		{name: "same-origin browser navigation", mutate: func(request *http.Request) {
			request.Header.Del("Origin")
			request.Header.Set("Sec-Fetch-Site", "same-origin")
			request.Header.Set("Sec-Fetch-Mode", "navigate")
			request.Header.Set("Sec-Fetch-Dest", "document")
		}, wantStatus: http.StatusOK},
		{name: "cross-site browser navigation", mutate: func(request *http.Request) {
			request.Header.Del("Origin")
			request.Header.Set("Sec-Fetch-Site", "cross-site")
			request.Header.Set("Sec-Fetch-Mode", "navigate")
			request.Header.Set("Sec-Fetch-Dest", "document")
		}, wantStatus: http.StatusForbidden},
		{name: "browser cannot claim native readonly", mutate: func(request *http.Request) {
			request.Header.Del("Origin")
			request.Header.Set(NonBrowserHeader, NonBrowserReadOnly)
			request.Header.Set("Sec-Fetch-Site", "same-origin")
		}, wantStatus: http.StatusForbidden},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := active.request(t, http.MethodGet, HealthPath, nil)
			if test.mutate != nil {
				test.mutate(request)
			}
			response, err := active.client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != test.wantStatus {
				body, _ := io.ReadAll(response.Body)
				t.Fatalf("status = %d, want %d, body = %q", response.StatusCode, test.wantStatus, body)
			}
			assertSecurityHeaders(t, response.Header)
		})
	}
}

func TestRawTCPRejectsDNSRebindingHostWithoutLeakingIt(t *testing.T) {
	active := startTestServer(t, nil, Config{})
	connection, err := net.DialTimeout("tcp4", active.server.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(2 * time.Second))
	_, err = fmt.Fprintf(connection, "GET %s HTTP/1.1\r\nHost: evil.example\r\nOrigin: %s\r\nConnection: close\r\n\r\n", HealthPath, active.server.Origin())
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(connection), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusForbidden || bytes.Contains(body, []byte("evil.example")) {
		t.Fatalf("status/body = %d %q", response.StatusCode, body)
	}
	assertSecurityHeaders(t, response.Header)
}

func TestBootstrapIsSingleUseAndCookieIsHardened(t *testing.T) {
	active := startTestServer(t, nil, Config{})

	invalid := active.request(t, http.MethodPost, BootstrapExchangePath, nil)
	invalid.Header.Set(BootstrapHeader, strings.Repeat("A", 43))
	invalidResponse, err := active.client.Do(invalid)
	if err != nil {
		t.Fatal(err)
	}
	invalidResponse.Body.Close()
	if invalidResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("invalid token status = %d", invalidResponse.StatusCode)
	}

	session := active.exchange(t)
	if session.cookie.Name != SessionCookieName || !session.cookie.HttpOnly || session.cookie.SameSite != http.SameSiteStrictMode || session.cookie.Path != "/" || session.cookie.Domain != "" {
		t.Fatalf("session cookie = %#v", session.cookie)
	}
	if session.cookie.Value == "" || session.csrf == "" || session.cookie.Value == session.csrf {
		t.Fatal("session or CSRF credential was missing or reused")
	}

	replay := active.request(t, http.MethodPost, BootstrapExchangePath, nil)
	replay.Header.Set(BootstrapHeader, active.bootstrap.HeaderValue())
	replayResponse, err := active.client.Do(replay)
	if err != nil {
		t.Fatal(err)
	}
	defer replayResponse.Body.Close()
	body, _ := io.ReadAll(replayResponse.Body)
	if replayResponse.StatusCode != http.StatusUnauthorized || bytes.Contains(body, []byte(active.bootstrap.HeaderValue())) {
		t.Fatalf("replay status/body = %d %q", replayResponse.StatusCode, body)
	}
}

func TestConcurrentBootstrapExchangeAllowsExactlyOne(t *testing.T) {
	active := startTestServer(t, nil, Config{})
	const attempts = 24
	statuses := make(chan int, attempts)
	errorsFound := make(chan error, attempts)
	var wait sync.WaitGroup
	start := make(chan struct{})
	for range attempts {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			request, err := http.NewRequest(http.MethodPost, active.server.Origin()+BootstrapExchangePath, nil)
			if err != nil {
				errorsFound <- err
				return
			}
			request.Header.Set("Origin", active.server.Origin())
			request.Header.Set(BootstrapHeader, active.bootstrap.HeaderValue())
			response, err := active.client.Do(request)
			if err != nil {
				errorsFound <- err
				return
			}
			_, _ = io.Copy(io.Discard, response.Body)
			response.Body.Close()
			statuses <- response.StatusCode
		}()
	}
	close(start)
	wait.Wait()
	close(statuses)
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatal(err)
		}
	}
	successes := 0
	unauthorized := 0
	for status := range statuses {
		switch status {
		case http.StatusOK:
			successes++
		case http.StatusUnauthorized:
			unauthorized++
		default:
			t.Fatalf("unexpected bootstrap status %d", status)
		}
	}
	if successes != 1 || unauthorized != attempts-1 {
		t.Fatalf("successes/unauthorized = %d/%d", successes, unauthorized)
	}
}

func TestBusinessSessionCSRFAndOriginEnforcement(t *testing.T) {
	router := NewRouter()
	var mutations atomic.Int32
	if err := router.HandleFunc(http.MethodGet, "/api/value", func(response http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(response, "ok")
	}); err != nil {
		t.Fatal(err)
	}
	if err := router.HandleFunc(http.MethodPost, "/api/mutate", func(response http.ResponseWriter, _ *http.Request) {
		mutations.Add(1)
		response.WriteHeader(http.StatusNoContent)
	}); err != nil {
		t.Fatal(err)
	}
	active := startTestServer(t, router, Config{})
	session := active.exchange(t)

	tests := []struct {
		name       string
		method     string
		path       string
		prepare    func(*http.Request)
		wantStatus int
	}{
		{name: "missing session", method: http.MethodGet, path: "/api/value", wantStatus: http.StatusUnauthorized},
		{name: "forged session", method: http.MethodGet, path: "/api/value", prepare: func(request *http.Request) {
			request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: strings.Repeat("A", 43)})
		}, wantStatus: http.StatusUnauthorized},
		{name: "valid read", method: http.MethodGet, path: "/api/value", prepare: func(request *http.Request) {
			request.AddCookie(session.cookie)
		}, wantStatus: http.StatusOK},
		{name: "missing csrf", method: http.MethodPost, path: "/api/mutate", prepare: func(request *http.Request) {
			request.AddCookie(session.cookie)
		}, wantStatus: http.StatusForbidden},
		{name: "forged csrf", method: http.MethodPost, path: "/api/mutate", prepare: func(request *http.Request) {
			request.AddCookie(session.cookie)
			request.Header.Set(CSRFHeader, strings.Repeat("A", 43))
		}, wantStatus: http.StatusForbidden},
		{name: "cross origin with credentials", method: http.MethodPost, path: "/api/mutate", prepare: func(request *http.Request) {
			request.AddCookie(session.cookie)
			request.Header.Set(CSRFHeader, session.csrf)
			request.Header.Set("Origin", "http://attacker.invalid")
		}, wantStatus: http.StatusForbidden},
		{name: "valid mutation", method: http.MethodPost, path: "/api/mutate", prepare: func(request *http.Request) {
			request.AddCookie(session.cookie)
			request.Header.Set(CSRFHeader, session.csrf)
		}, wantStatus: http.StatusNoContent},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := active.request(t, test.method, test.path, nil)
			if test.prepare != nil {
				test.prepare(request)
			}
			response, err := active.client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.StatusCode, test.wantStatus)
			}
		})
	}
	if mutations.Load() != 1 {
		t.Fatalf("mutation handler calls = %d, want 1", mutations.Load())
	}
}

func TestBusinessErrorsAreSanitizedAndBodiesAreBounded(t *testing.T) {
	router := NewRouter()
	if err := router.HandleFunc(http.MethodPost, "/api/fail", func(response http.ResponseWriter, request *http.Request) {
		body, readErr := io.ReadAll(request.Body)
		if readErr != nil || len(body) > 8 {
			response.Header().Set("X-Internal-Debug", "database-password")
			http.Error(response, "database-password and stack trace", http.StatusInternalServerError)
			return
		}
		response.WriteHeader(http.StatusNoContent)
	}); err != nil {
		t.Fatal(err)
	}
	if err := router.HandleFunc(http.MethodGet, "/api/panic", func(response http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(response, "partial secret")
		panic("internal panic secret")
	}); err != nil {
		t.Fatal(err)
	}
	if err := router.HandleFunc(http.MethodGet, "/api/overflow", func(response http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(response, "123456789")
	}); err != nil {
		t.Fatal(err)
	}
	active := startTestServer(t, router, Config{MaxBodyBytes: 8, MaxResponseBytes: 8})
	session := active.exchange(t)

	smallRequest := active.request(t, http.MethodPost, "/api/fail", strings.NewReader("x"))
	smallRequest.AddCookie(session.cookie)
	smallRequest.Header.Set(CSRFHeader, session.csrf)
	smallResponse, err := active.client.Do(smallRequest)
	if err != nil {
		t.Fatal(err)
	}
	smallBody, _ := io.ReadAll(smallResponse.Body)
	smallResponse.Body.Close()
	if smallResponse.StatusCode != http.StatusNoContent {
		t.Fatalf("small request status/body = %d %q", smallResponse.StatusCode, smallBody)
	}

	// A handler-originated failure cannot return its diagnostic header or body.
	leakRequest := active.request(t, http.MethodPost, "/api/fail", strings.NewReader("123456789"))
	leakRequest.ContentLength = -1
	leakRequest.TransferEncoding = []string{"chunked"}
	leakRequest.AddCookie(session.cookie)
	leakRequest.Header.Set(CSRFHeader, session.csrf)
	leakResponse, err := active.client.Do(leakRequest)
	if err != nil {
		t.Fatal(err)
	}
	leakBody, _ := io.ReadAll(leakResponse.Body)
	leakResponse.Body.Close()
	if leakResponse.StatusCode != http.StatusInternalServerError || bytes.Contains(leakBody, []byte("database-password")) || leakResponse.Header.Get("X-Internal-Debug") != "" {
		t.Fatalf("handler error leaked internals: status=%d headers=%v body=%q", leakResponse.StatusCode, leakResponse.Header, leakBody)
	}

	request := active.request(t, http.MethodPost, "/api/fail", strings.NewReader("too large for limit"))
	request.AddCookie(session.cookie)
	request.Header.Set(CSRFHeader, session.csrf)
	response, err := active.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, body = %q", response.StatusCode, body)
	}
	if bytes.Contains(body, []byte("database-password")) || response.Header.Get("X-Internal-Debug") != "" {
		t.Fatalf("error leaked handler internals: headers=%v body=%q", response.Header, body)
	}

	for _, routePath := range []string{"/api/panic", "/api/overflow"} {
		boundedRequest := active.request(t, http.MethodGet, routePath, nil)
		boundedRequest.AddCookie(session.cookie)
		boundedResponse, err := active.client.Do(boundedRequest)
		if err != nil {
			t.Fatal(err)
		}
		boundedBody, _ := io.ReadAll(boundedResponse.Body)
		boundedResponse.Body.Close()
		if boundedResponse.StatusCode != http.StatusInternalServerError || bytes.Contains(boundedBody, []byte("secret")) || bytes.Contains(boundedBody, []byte("123456789")) {
			t.Fatalf("%s status/body = %d %q", routePath, boundedResponse.StatusCode, boundedBody)
		}
	}
}

func TestRequestTimeoutIsBoundedAndSafe(t *testing.T) {
	router := NewRouter()
	if err := router.HandleFunc(http.MethodGet, "/api/slow", func(response http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
		time.Sleep(5 * time.Millisecond)
		_, _ = io.WriteString(response, "secret after timeout")
	}); err != nil {
		t.Fatal(err)
	}
	active := startTestServer(t, router, Config{RequestTimeout: 20 * time.Millisecond})
	session := active.exchange(t)
	request := active.request(t, http.MethodGet, "/api/slow", nil)
	request.AddCookie(session.cookie)
	response, err := active.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusServiceUnavailable || bytes.Contains(body, []byte("secret")) {
		t.Fatalf("timeout status/body = %d %q", response.StatusCode, body)
	}
	assertSecurityHeaders(t, response.Header)
}

func TestConfigurationHardLimitsAndNilShutdownContext(t *testing.T) {
	if _, _, err := Start(nil, Config{MaxBodyBytes: maximumBodyBytes + 1}); err == nil {
		t.Fatal("Start accepted body limit above hard maximum")
	}
	if _, _, err := Start(nil, Config{MaxResponseBytes: maximumResponse + 1}); err == nil {
		t.Fatal("Start accepted response limit above hard maximum")
	}
	if _, _, err := Start(nil, Config{RequestTimeout: 2*time.Minute + time.Nanosecond}); err == nil {
		t.Fatal("Start accepted request timeout above hard maximum")
	}
	active := startTestServer(t, nil, Config{})
	if err := active.server.Shutdown(nil); err == nil {
		t.Fatal("Shutdown(nil) succeeded")
	}
}

func TestShutdownInvalidatesSessionAndStopsListener(t *testing.T) {
	router := NewRouter()
	if err := router.HandleFunc(http.MethodGet, "/api/value", func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusNoContent)
	}); err != nil {
		t.Fatal(err)
	}
	active := startTestServer(t, router, Config{})
	_ = active.exchange(t)
	address := active.server.Addr().String()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := active.server.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := active.server.Shutdown(ctx); err != nil {
		t.Fatalf("second Shutdown() error = %v", err)
	}
	select {
	case err := <-active.server.Done():
		if err != nil {
			t.Fatalf("Done() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not finish shutdown")
	}
	connection, err := net.DialTimeout("tcp4", address, 100*time.Millisecond)
	if err == nil {
		connection.Close()
		t.Fatal("listener still accepted connections after Shutdown")
	}
}

func assertSecurityHeaders(t *testing.T, header http.Header) {
	t.Helper()
	checks := map[string]string{
		"Cache-Control":           "no-store",
		"Content-Security-Policy": "default-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'",
		"Referrer-Policy":         "no-referrer",
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
	}
	for name, want := range checks {
		if got := header.Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}
