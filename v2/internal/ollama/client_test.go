package ollama

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestProbeAndGenerate(t *testing.T) {
	t.Parallel()
	var chatCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if encoding := request.Header.Get("Accept-Encoding"); encoding != "identity" {
			t.Errorf("Accept-Encoding = %q; want identity", encoding)
		}
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		switch request.URL.Path {
		case "/api/tags":
			fmt.Fprint(writer, `{"models":[{"name":"qwen3:8b","modified_at":"2026-08-23T00:00:00Z","size":123,"digest":"abc","details":{"family":"qwen3"}}],"extension":{"supported":true,"Feature":"upper","feature":"lower"}}`)
		case "/api/chat":
			chatCalls.Add(1)
			var body struct {
				Model    string `json:"model"`
				Messages []struct {
					Role, Content string
				} `json:"messages"`
				Stream bool `json:"stream"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Errorf("decode request: %v", err)
			}
			if body.Model != "qwen3:8b" || body.Stream || len(body.Messages) != 1 || body.Messages[0].Content != "问题" {
				t.Errorf("request = %#v", body)
			}
			fmt.Fprint(writer, `{"message":{"role":"assistant","content":"回答"},"done":true,"eval_count":2}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, time.Second)
	names, err := client.Probe(context.Background())
	if err != nil || len(names) != 1 || names[0] != "qwen3:8b" {
		t.Fatalf("Probe = %v, %v", names, err)
	}
	answer, err := client.Generate(context.Background(), "问题")
	if err != nil || answer != "回答" || chatCalls.Load() != 1 {
		t.Fatalf("Generate = %q, %v; calls=%d", answer, err, chatCalls.Load())
	}
	if client.transport.Proxy != nil {
		t.Fatal("transport unexpectedly configured a proxy")
	}
	if !client.transport.DisableCompression {
		t.Fatal("transport unexpectedly permits automatic decompression")
	}
	if client.transport.MaxResponseHeaderBytes != maxResponseHeaderBytes {
		t.Fatalf("MaxResponseHeaderBytes = %d", client.transport.MaxResponseHeaderBytes)
	}
}

func TestNewRejectsUnsafeOrigins(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		"", "http://localhost:11434", "http://192.168.1.2:11434",
		"https://127.0.0.1:11434", "http://127.0.0.1",
		"http://user@127.0.0.1:11434", "http://127.0.0.1:11434/path",
		"http://127.0.0.1:11434?x=1", "http://[::ffff:127.0.0.1]:11434",
	} {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			if _, err := New(Options{BaseURL: raw, Model: "model"}); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("New(%q) error = %v; want ErrInvalidConfig", raw, err)
			}
		})
	}
}

func TestNewAcceptsLiteralIPv6Loopback(t *testing.T) {
	t.Parallel()
	client, err := New(Options{BaseURL: "http://[::1]:11434", Model: "model"})
	if err != nil {
		t.Fatalf("New IPv6 loopback: %v", err)
	}
	defer client.CloseIdleConnections()
	if client.baseURL != "http://[::1]:11434" {
		t.Fatalf("base URL = %q", client.baseURL)
	}
}

func TestProtocolLimitsRedirectAndTimeout(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		handler http.HandlerFunc
		timeout time.Duration
		want    error
	}{
		{
			name: "redirect",
			handler: func(writer http.ResponseWriter, request *http.Request) {
				http.Redirect(writer, request, "/elsewhere", http.StatusFound)
			},
			want: ErrProtocol,
		},
		{
			name: "content type",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "text/plain")
				fmt.Fprint(writer, `{}`)
			},
			want: ErrProtocol,
		},
		{
			name: "malformed",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				fmt.Fprint(writer, `{`)
			},
			want: ErrProtocol,
		},
		{
			name: "oversized",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				fmt.Fprint(writer, `{"padding":"`+strings.Repeat("x", maxResponseBytes)+`"}`)
			},
			want: ErrResponseTooLarge,
		},
		{
			name: "timeout",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				time.Sleep(100 * time.Millisecond)
				writer.Header().Set("Content-Type", "application/json")
				fmt.Fprint(writer, `{"models":[]}`)
			},
			timeout: 10 * time.Millisecond,
			want:    context.DeadlineExceeded,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(test.handler)
			defer server.Close()
			timeout := test.timeout
			if timeout == 0 {
				timeout = time.Second
			}
			client := newTestClient(t, server.URL, timeout)
			_, err := client.Probe(context.Background())
			if !errors.Is(err, test.want) {
				t.Fatalf("Probe error = %v; want %v", err, test.want)
			}
		})
	}
}

func TestGenerateBoundsAndCancellation(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	defer server.Close()
	client := newTestClient(t, server.URL, time.Second)
	if _, err := client.Generate(context.Background(), strings.Repeat("x", MaxPromptBytes+1)); !errors.Is(err, ErrRequestTooLarge) {
		t.Fatalf("oversized prompt error = %v", err)
	}
	if _, err := client.Generate(context.Background(), string([]byte{0xff})); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("invalid UTF-8 prompt error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Generate(ctx, "prompt"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled error = %v", err)
	}
}

func TestGenerateExactPromptBoundAndCancellationWhileWaitingForHeaders(t *testing.T) {
	t.Run("exact prompt bound is sent once", func(t *testing.T) {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			calls.Add(1)
			var body struct {
				Messages []struct {
					Content string `json:"content"`
				} `json:"messages"`
			}
			decodeErr := json.NewDecoder(request.Body).Decode(&body)
			messageCount := len(body.Messages)
			promptBytes := 0
			if messageCount == 1 {
				promptBytes = len(body.Messages[0].Content)
			}
			if decodeErr != nil || messageCount != 1 || promptBytes != MaxPromptBytes {
				t.Errorf("exact-bound request decode/messages/prompt-bytes = %v/%d/%d", decodeErr, messageCount, promptBytes)
			}
			writer.Header().Set("Content-Type", "application/json")
			fmt.Fprint(writer, `{"message":{"role":"assistant","content":"bounded"},"done":true}`)
		}))
		defer server.Close()
		client := newTestClient(t, server.URL, time.Second)
		answer, err := client.Generate(context.Background(), strings.Repeat("x", MaxPromptBytes))
		if err != nil || answer != "bounded" || calls.Load() != 1 {
			t.Fatalf("exact-bound Generate = %q, %v; calls=%d", answer, err, calls.Load())
		}
		if _, err := client.Generate(context.Background(), strings.Repeat("x", MaxPromptBytes+1)); !errors.Is(err, ErrRequestTooLarge) || calls.Load() != 1 {
			t.Fatalf("over-bound Generate error/calls = %v/%d", err, calls.Load())
		}
	})

	t.Run("caller cancellation interrupts response headers", func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
			close(started)
			select {
			case <-request.Context().Done():
			case <-release:
			}
		}))
		defer server.Close()
		defer close(release)
		client := newTestClient(t, server.URL, time.Second)
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		const promptCanary = "HEADER-CANCEL-PROMPT-CANARY-36cc0c"
		go func() {
			_, err := client.Generate(ctx, promptCanary)
			result <- err
		}()
		select {
		case <-started:
			cancel()
		case <-time.After(time.Second):
			cancel()
			t.Fatal("request did not reach the response-header boundary")
		}
		select {
		case err := <-result:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("header cancellation error = %v", err)
			}
			if strings.Contains(err.Error(), promptCanary) {
				t.Fatalf("header cancellation leaked prompt: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("Generate remained blocked after caller cancellation")
		}
	})
}

func TestProbeUsesLiteralIPv6Loopback(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback is unavailable: %v", err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"models":[]}`)
	})}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() {
		_ = server.Close()
		select {
		case serveErr := <-done:
			if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
				t.Errorf("serve IPv6 loopback: %v", serveErr)
			}
		case <-time.After(time.Second):
			t.Error("IPv6 loopback server did not stop")
		}
	})

	client := newTestClient(t, "http://"+listener.Addr().String(), time.Second)
	models, err := client.Probe(context.Background())
	if err != nil || len(models) != 0 {
		t.Fatalf("Probe IPv6 loopback = %v, %v", models, err)
	}
}

func TestDialPolicyRejectsDNSAndUnexpectedTargets(t *testing.T) {
	t.Parallel()
	client, err := New(Options{BaseURL: "http://127.0.0.1:11434", Model: "model", Timeout: time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer client.CloseIdleConnections()

	for _, attempt := range []struct {
		network string
		address string
	}{
		{network: "tcp", address: "localhost:11434"},
		{network: "tcp", address: "127.0.0.2:11434"},
		{network: "udp", address: "127.0.0.1:11434"},
	} {
		if connection, err := client.transport.DialContext(context.Background(), attempt.network, attempt.address); err == nil {
			_ = connection.Close()
			t.Fatalf("DialContext(%q, %q) unexpectedly succeeded", attempt.network, attempt.address)
		}
	}
}

func TestLiteralTransportIgnoresAmbientProxyAndDNS(t *testing.T) {
	var proxyCalls atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		proxyCalls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"models":[{"name":"proxied"}]}`)
	}))
	defer proxy.Close()
	var originCalls atomic.Int32
	origin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		originCalls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"models":[{"name":"direct"}]}`)
	}))
	defer origin.Close()

	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("ALL_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "")
	var resolverCalls atomic.Int32
	previousResolver := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(context.Context, string, string) (net.Conn, error) {
			resolverCalls.Add(1)
			return nil, errors.New("hostile resolver must not be called")
		},
	}
	t.Cleanup(func() { net.DefaultResolver = previousResolver })

	client := newTestClient(t, origin.URL, time.Second)
	models, err := client.Probe(context.Background())
	if err != nil || len(models) != 1 || models[0] != "direct" {
		t.Fatalf("Probe = %v, %v; want direct origin", models, err)
	}
	if calls := proxyCalls.Load(); calls != 0 {
		t.Fatalf("ambient proxy received %d call(s)", calls)
	}
	if calls := resolverCalls.Load(); calls != 0 {
		t.Fatalf("resolver received %d call(s) for literal IP", calls)
	}
}

func TestRedirectNeverDialsTarget(t *testing.T) {
	t.Parallel()
	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		targetCalls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"models":[{"name":"redirected"}]}`)
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, target.URL+"/api/tags", http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	client := newTestClient(t, source.URL, time.Second)
	if _, err := client.Probe(context.Background()); !errors.Is(err, ErrProtocol) {
		t.Fatalf("Probe redirect error = %v; want ErrProtocol", err)
	}
	if calls := targetCalls.Load(); calls != 0 {
		t.Fatalf("redirect target was called %d time(s)", calls)
	}
}

func TestUntrustedJSONFailsClosed(t *testing.T) {
	t.Parallel()
	const responseCanary = "RESPONSE-CANARY-6f690bf8"
	deepExtension := `{"models":[],"extension":` + strings.Repeat("[", maxJSONNestingDepth+1) + `0` + strings.Repeat("]", maxJSONNestingDepth+1) + `}`
	tests := []struct {
		name     string
		response string
		chat     bool
	}{
		{name: "duplicate top-level field", response: `{"models":[],"models":[{"name":"` + responseCanary + `"}]}`},
		{name: "duplicate model field", response: `{"models":[{"name":"safe","name":"` + responseCanary + `"}]}`},
		{name: "duplicate unknown extension field", response: `{"models":[],"extension":{"value":"safe","value":"` + responseCanary + `"}}`},
		{name: "duplicate chat completion field", response: `{"message":{"role":"assistant","content":"safe"},"done":false,"done":true}`, chat: true},
		{name: "duplicate chat message field", response: `{"message":{"role":"assistant","content":"ignored","content":"safe"},"done":true}`, chat: true},
		{name: "mis-cased models field only", response: `{"Models":[]}`},
		{name: "mis-cased models field", response: `{"models":[],"Models":[{"name":"` + responseCanary + `"}]}`},
		{name: "mis-cased model name field only", response: `{"models":[{"Name":"safe"}]}`},
		{name: "mis-cased model name field", response: `{"models":[{"name":"safe","Name":"` + responseCanary + `"}]}`},
		{name: "mis-cased chat message field only", response: `{"Message":{"role":"assistant","content":"safe"},"done":true}`, chat: true},
		{name: "mis-cased chat completion field", response: `{"message":{"role":"assistant","content":"safe"},"done":false,"Done":true}`, chat: true},
		{name: "mis-cased chat message field", response: `{"message":{"role":"user","Role":"assistant","content":"` + responseCanary + `"},"done":true}`, chat: true},
		{name: "mis-cased chat content field only", response: `{"message":{"role":"assistant","Content":"safe"},"done":true}`, chat: true},
		{name: "excessive unknown extension nesting", response: deepExtension},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				fmt.Fprint(writer, test.response)
			}))
			defer server.Close()
			client := newTestClient(t, server.URL, time.Second)
			var err error
			if test.chat {
				_, err = client.Generate(context.Background(), "safe prompt")
			} else {
				_, err = client.Probe(context.Background())
			}
			if !errors.Is(err, ErrProtocol) {
				t.Fatalf("response error = %v; want ErrProtocol", err)
			}
			if strings.Contains(err.Error(), responseCanary) {
				t.Fatalf("error leaked response content: %v", err)
			}
		})
	}
}

func TestUntrustedJSONAggregateLimits(t *testing.T) {
	t.Parallel()
	objectResponse := func(properties int, value string) string {
		var builder strings.Builder
		builder.WriteString(`{"models":[],"extension":{`)
		for index := 0; index < properties; index++ {
			if index != 0 {
				builder.WriteByte(',')
			}
			builder.WriteString(`"k`)
			builder.WriteString(strconv.Itoa(index))
			builder.WriteString(`":`)
			builder.WriteString(value)
		}
		builder.WriteString(`}}`)
		return builder.String()
	}
	arrayResponse := func(items int) string {
		var builder strings.Builder
		builder.WriteString(`{"models":[],"extension":[`)
		for index := 0; index < items; index++ {
			if index != 0 {
				builder.WriteByte(',')
			}
			builder.WriteByte('0')
		}
		builder.WriteString(`]}`)
		return builder.String()
	}
	tests := []struct {
		name       string
		response   string
		wantDetail string
	}{
		{name: "object keys", response: objectResponse(maxJSONKeys+1, "0"), wantDetail: "object key limit"},
		{name: "array items", response: arrayResponse(maxJSONItems + 1), wantDetail: "array item limit"},
		{name: "tokens", response: objectResponse(maxJSONTokens/5+1, "[{}]"), wantDetail: "token limit"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if len(test.response) > maxResponseBytes {
				t.Fatalf("amplified response is %d bytes; must exercise the JSON budget below the byte limit", len(test.response))
			}
			if err := rejectDuplicateJSONKeys([]byte(test.response)); err == nil || !strings.Contains(err.Error(), test.wantDetail) {
				t.Fatalf("JSON scan error = %v; want %q", err, test.wantDetail)
			}
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				fmt.Fprint(writer, test.response)
			}))
			defer server.Close()
			client := newTestClient(t, server.URL, time.Second)
			if _, err := client.Probe(context.Background()); !errors.Is(err, ErrProtocol) {
				t.Fatalf("Probe aggregate limit error = %v; want ErrProtocol", err)
			}
		})
	}
}

func TestResponseMetadataAndFramingFailClosed(t *testing.T) {
	t.Parallel()
	const responseCanary = "RESPONSE-CANARY-df9c59db"
	var compressed bytes.Buffer
	zipper := gzip.NewWriter(&compressed)
	if _, err := io.WriteString(zipper, `{"models":[],"padding":"`+strings.Repeat("x", maxResponseBytes*2)+responseCanary+`"}`); err != nil {
		t.Fatalf("build gzip bomb: %v", err)
	}
	if err := zipper.Close(); err != nil {
		t.Fatalf("close gzip bomb: %v", err)
	}

	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    error
	}{
		{
			name: "duplicate content type",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Add("Content-Type", "application/json")
				writer.Header().Add("Content-Type", "application/json; note="+responseCanary)
				fmt.Fprint(writer, `{"models":[]}`)
			},
			want: ErrProtocol,
		},
		{
			name: "non UTF-8 charset",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json; charset=iso-8859-1")
				fmt.Fprint(writer, `{"models":[]}`)
			},
			want: ErrProtocol,
		},
		{
			name: "unexpected media type parameter",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json; profile="+responseCanary)
				fmt.Fprint(writer, `{"models":[]}`)
			},
			want: ErrProtocol,
		},
		{
			name: "gzip decompression bomb",
			handler: func(writer http.ResponseWriter, request *http.Request) {
				if encoding := request.Header.Get("Accept-Encoding"); encoding != "identity" {
					t.Errorf("Accept-Encoding = %q; want identity", encoding)
				}
				writer.Header().Set("Content-Type", "application/json")
				writer.Header().Set("Content-Encoding", "gzip")
				_, _ = writer.Write(compressed.Bytes())
			},
			want: ErrProtocol,
		},
		{
			name: "declared oversized body",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.Header().Set("Content-Length", fmt.Sprint(maxResponseBytes+1))
				writer.WriteHeader(http.StatusOK)
			},
			want: ErrResponseTooLarge,
		},
		{
			name: "truncated declared body",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.Header().Set("Content-Length", "64")
				writer.WriteHeader(http.StatusOK)
				fmt.Fprint(writer, `{"models":[]}`)
			},
			want: ErrProtocol,
		},
		{
			name: "oversized response headers",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.Header().Set("X-Response-Canary", responseCanary+strings.Repeat("x", maxResponseHeaderBytes+1))
				fmt.Fprint(writer, `{"models":[]}`)
			},
			want: ErrUnavailable,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(test.handler)
			defer server.Close()
			client := newTestClient(t, server.URL, time.Second)
			_, err := client.Probe(context.Background())
			if !errors.Is(err, test.want) {
				t.Fatalf("Probe error = %v; want %v", err, test.want)
			}
			if strings.Contains(err.Error(), responseCanary) {
				t.Fatalf("error leaked response metadata or content: %v", err)
			}
		})
	}
}

func TestSlowBodyTimeoutAndCancellation(t *testing.T) {
	t.Parallel()
	newSlowBodyServer := func(t *testing.T, started chan<- struct{}) *httptest.Server {
		t.Helper()
		return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusOK)
			fmt.Fprint(writer, `{"models":[`)
			writer.(http.Flusher).Flush()
			started <- struct{}{}
			select {
			case <-request.Context().Done():
			case <-time.After(time.Second):
			}
		}))
	}

	t.Run("client timeout covers response body", func(t *testing.T) {
		started := make(chan struct{}, 1)
		server := newSlowBodyServer(t, started)
		defer server.Close()
		client := newTestClient(t, server.URL, 25*time.Millisecond)
		_, err := client.Probe(context.Background())
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Probe slow body error = %v; want deadline exceeded", err)
		}
	})

	t.Run("caller cancellation interrupts response body", func(t *testing.T) {
		const promptCanary = "PROMPT-CANARY-66fbf4ae"
		started := make(chan struct{}, 1)
		server := newSlowBodyServer(t, started)
		defer server.Close()
		client := newTestClient(t, server.URL, time.Second)
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		go func() {
			_, err := client.Generate(ctx, promptCanary)
			result <- err
		}()
		select {
		case <-started:
			cancel()
		case <-time.After(time.Second):
			cancel()
			t.Fatal("server did not start response body")
		}
		select {
		case err := <-result:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Generate cancellation error = %v; want context.Canceled", err)
			}
			if strings.Contains(err.Error(), promptCanary) {
				t.Fatalf("cancellation error leaked prompt: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("Generate did not return after cancellation")
		}
	})
}

func TestConnectionReuseAndEarlyFailureDiscard(t *testing.T) {
	t.Parallel()
	t.Run("complete responses reuse a connection", func(t *testing.T) {
		var connections atomic.Int32
		server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			fmt.Fprint(writer, `{"models":[]}`)
		}))
		server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
			if state == http.StateNew {
				connections.Add(1)
			}
		}
		server.Start()
		defer server.Close()
		client := newTestClient(t, server.URL, time.Second)
		for attempt := 0; attempt < 2; attempt++ {
			if _, err := client.Probe(context.Background()); err != nil {
				t.Fatalf("Probe %d: %v", attempt+1, err)
			}
		}
		if count := connections.Load(); count != 1 {
			t.Fatalf("connections = %d; want one reused connection", count)
		}
	})

	t.Run("early protocol failure discards a connection", func(t *testing.T) {
		var calls atomic.Int32
		var connections atomic.Int32
		server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if calls.Add(1) == 1 {
				writer.Header().Set("Content-Type", "text/plain")
				writer.WriteHeader(http.StatusOK)
				fmt.Fprint(writer, "unfinished")
				writer.(http.Flusher).Flush()
				select {
				case <-request.Context().Done():
				case <-time.After(time.Second):
				}
				return
			}
			writer.Header().Set("Content-Type", "application/json")
			fmt.Fprint(writer, `{"models":[]}`)
		}))
		server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
			if state == http.StateNew {
				connections.Add(1)
			}
		}
		server.Start()
		defer server.Close()
		client := newTestClient(t, server.URL, time.Second)
		if _, err := client.Probe(context.Background()); !errors.Is(err, ErrProtocol) {
			t.Fatalf("first Probe error = %v; want ErrProtocol", err)
		}
		if _, err := client.Probe(context.Background()); err != nil {
			t.Fatalf("second Probe: %v", err)
		}
		if count := connections.Load(); count != 2 {
			t.Fatalf("connections = %d; want failed response connection discarded", count)
		}
	})
}

func TestMalformedHTTPAndFramingErrorsAreSanitized(t *testing.T) {
	t.Parallel()
	const (
		promptCanary   = "PROMPT-CANARY-2d58ff61"
		responseCanary = "RESPONSE-CANARY-b6da0b09"
	)
	tests := []struct {
		name     string
		response string
		chat     bool
		want     error
	}{
		{
			name:     "malformed status line",
			response: "HTTP/1.1 " + responseCanary + "\r\nConnection: close\r\n\r\n",
			chat:     true,
			want:     ErrUnavailable,
		},
		{
			name: "conflicting content lengths",
			response: "HTTP/1.1 200 OK\r\n" +
				"Content-Type: application/json\r\n" +
				"Content-Length: 13\r\n" +
				"Content-Length: 14\r\n" +
				"X-Response-Canary: " + responseCanary + "\r\n" +
				"Connection: close\r\n\r\n{\"models\":[]}",
			want: ErrUnavailable,
		},
		{
			name: "truncated chunked trailer",
			response: "HTTP/1.1 200 OK\r\n" +
				"Content-Type: application/json\r\n" +
				"Transfer-Encoding: chunked\r\n" +
				"Connection: close\r\n\r\n" +
				"d\r\n{\"models\":[]}\r\n0\r\n",
			want: ErrProtocol,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := newTestClient(t, rawServerURL(t, test.response), time.Second)
			var err error
			if test.chat {
				_, err = client.Generate(context.Background(), promptCanary)
			} else {
				_, err = client.Probe(context.Background())
			}
			if !errors.Is(err, test.want) {
				t.Fatalf("request error = %v; want %v", err, test.want)
			}
			if strings.Contains(err.Error(), promptCanary) || strings.Contains(err.Error(), responseCanary) {
				t.Fatalf("error leaked prompt or response diagnostics: %v", err)
			}
		})
	}
}

func rawServerURL(t *testing.T, response string) string {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen raw server: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		connection, acceptErr := listener.Accept()
		_ = listener.Close()
		if acceptErr != nil {
			return
		}
		defer connection.Close()
		_ = connection.SetDeadline(time.Now().Add(time.Second))
		requestPrefix := make([]byte, 4096)
		_, _ = connection.Read(requestPrefix)
		_, _ = io.WriteString(connection, response)
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("raw server did not stop")
		}
	})
	return "http://" + listener.Addr().String()
}

func newTestClient(t *testing.T, rawURL string, timeout time.Duration) *Client {
	t.Helper()
	client, err := New(Options{BaseURL: rawURL, Model: "qwen3:8b", Timeout: timeout})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(client.CloseIdleConnections)
	return client
}
