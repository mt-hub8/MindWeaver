package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestProbeAndGenerate(t *testing.T) {
	t.Parallel()
	var chatCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		switch request.URL.Path {
		case "/api/tags":
			fmt.Fprint(writer, `{"models":[{"name":"qwen3:8b"}]}`)
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

func newTestClient(t *testing.T, rawURL string, timeout time.Duration) *Client {
	t.Helper()
	client, err := New(Options{BaseURL: rawURL, Model: "qwen3:8b", Timeout: timeout})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(client.CloseIdleConnections)
	return client
}
