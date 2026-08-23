// Package ollama is the first release's concrete, loopback-only Ollama client.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxPromptBytes   = 64 << 10
	maxRequestBytes  = 128 << 10
	maxResponseBytes = 4 << 20
	maxModels        = 1000
	defaultTimeout   = 60 * time.Second
)

var (
	ErrInvalidConfig    = errors.New("ollama: invalid configuration")
	ErrInvalidRequest   = errors.New("ollama: invalid request")
	ErrRequestTooLarge  = errors.New("ollama: request exceeds size limit")
	ErrResponseTooLarge = errors.New("ollama: response exceeds size limit")
	ErrProtocol         = errors.New("ollama: invalid response protocol")
	ErrUnavailable      = errors.New("ollama: service unavailable")
	errRedirect         = errors.New("ollama: redirects are not allowed")
)

// Options contains the only configurable Ollama values in the first release.
type Options struct {
	BaseURL string
	Model   string
	Timeout time.Duration
}

// Client dials one fixed, literal loopback address. It never uses ambient proxy
// settings or follows redirects.
type Client struct {
	baseURL   string
	model     string
	http      *http.Client
	transport *http.Transport
}

func New(options Options) (*Client, error) {
	baseURL, dialAddress, err := validateBaseURL(options.BaseURL)
	if err != nil {
		return nil, err
	}
	model := strings.TrimSpace(options.Model)
	if model == "" || model != options.Model || len(model) > 255 || !utf8.ValidString(model) || strings.IndexFunc(model, isControl) >= 0 {
		return nil, fmt.Errorf("%w: model must contain 1 to 255 printable characters", ErrInvalidConfig)
	}
	timeout := options.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	if timeout < time.Millisecond || timeout > 10*time.Minute {
		return nil, fmt.Errorf("%w: timeout must be between 1ms and 10m", ErrInvalidConfig)
	}

	dialer := &net.Dialer{Timeout: min(timeout, 5*time.Second), KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:                 nil,
		DisableCompression:    true,
		ForceAttemptHTTP2:     false,
		MaxConnsPerHost:       4,
		MaxIdleConnsPerHost:   2,
		IdleConnTimeout:       30 * time.Second,
		ResponseHeaderTimeout: timeout,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			if network != "tcp" && network != "tcp4" && network != "tcp6" {
				return nil, fmt.Errorf("ollama: unsupported network %q", network)
			}
			if !sameAddress(address, dialAddress) {
				return nil, fmt.Errorf("ollama: refused unexpected dial target")
			}
			return dialer.DialContext(ctx, network, dialAddress)
		},
	}
	return &Client{
		baseURL:   baseURL,
		model:     model,
		transport: transport,
		http: &http.Client{
			Transport: transport,
			Timeout:   timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errRedirect
			},
		},
	}, nil
}

func (c *Client) CloseIdleConnections() {
	if c != nil && c.transport != nil {
		c.transport.CloseIdleConnections()
	}
}

// Probe returns the bounded model names reported by /api/tags.
func (c *Client) Probe(ctx context.Context) ([]string, error) {
	var response struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/api/tags", nil, &response); err != nil {
		return nil, err
	}
	if len(response.Models) > maxModels {
		return nil, fmt.Errorf("%w: too many models", ErrProtocol)
	}
	names := make([]string, 0, len(response.Models))
	for _, model := range response.Models {
		if model.Name == "" || len(model.Name) > 255 || strings.IndexFunc(model.Name, isControl) >= 0 {
			return nil, fmt.Errorf("%w: invalid model name", ErrProtocol)
		}
		names = append(names, model.Name)
	}
	return names, nil
}

// Generate performs one non-streaming chat request and returns only the model's
// answer text. Persistence and citation validation belong to the Ask workflow.
func (c *Client) Generate(ctx context.Context, prompt string) (string, error) {
	if len(prompt) == 0 || !utf8.ValidString(prompt) {
		return "", ErrInvalidRequest
	}
	if len(prompt) > MaxPromptBytes {
		return "", ErrRequestTooLarge
	}
	request := struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		Stream bool `json:"stream"`
	}{Model: c.model, Stream: false}
	request.Messages = append(request.Messages, struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}{Role: "user", Content: prompt})
	var response struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		Done bool `json:"done"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/api/chat", request, &response); err != nil {
		return "", err
	}
	if !response.Done || response.Message.Role != "assistant" || strings.TrimSpace(response.Message.Content) == "" {
		return "", fmt.Errorf("%w: incomplete chat response", ErrProtocol)
	}
	if len(response.Message.Content) > maxResponseBytes {
		return "", ErrResponseTooLarge
	}
	return response.Message.Content, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, input, output any) error {
	if c == nil || c.http == nil {
		return fmt.Errorf("%w: client is not initialized", ErrInvalidConfig)
	}
	if ctx == nil {
		return errors.New("ollama: nil context")
	}
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("ollama: encode request: %w", err)
		}
		if len(encoded) > maxRequestBytes {
			return ErrRequestTooLarge
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("ollama: create request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(request)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if errors.Is(err, errRedirect) {
			return ErrProtocol
		}
		if networkError, ok := err.(net.Error); ok && networkError.Timeout() {
			return context.DeadlineExceeded
		}
		return fmt.Errorf("%w: request failed", ErrUnavailable)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if response.StatusCode >= 500 {
			return fmt.Errorf("%w: HTTP %d", ErrUnavailable, response.StatusCode)
		}
		return fmt.Errorf("%w: HTTP %d", ErrProtocol, response.StatusCode)
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return fmt.Errorf("%w: response is not application/json", ErrProtocol)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("%w: response read failed", ErrUnavailable)
	}
	if len(data) > maxResponseBytes {
		return ErrResponseTooLarge
	}
	if !utf8.Valid(data) {
		return fmt.Errorf("%w: response is not valid UTF-8", ErrProtocol)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(output); err != nil {
		return fmt.Errorf("%w: malformed JSON", ErrProtocol)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: trailing JSON", ErrProtocol)
	}
	return nil
}

func validateBaseURL(raw string) (baseURL, dialAddress string, err error) {
	if strings.TrimSpace(raw) == "" || raw != strings.TrimSpace(raw) {
		return "", "", fmt.Errorf("%w: base URL is required without surrounding whitespace", ErrInvalidConfig)
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "http" || parsed.Opaque != "" || parsed.User != nil ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawPath != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", fmt.Errorf("%w: base URL must be an HTTP origin", ErrInvalidConfig)
	}
	host := parsed.Hostname()
	address, err := netip.ParseAddr(host)
	if err != nil || address.Zone() != "" || address.Is4In6() || !address.IsLoopback() {
		return "", "", fmt.Errorf("%w: base URL host must be a literal loopback address", ErrInvalidConfig)
	}
	portText := parsed.Port()
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return "", "", fmt.Errorf("%w: base URL requires an explicit port", ErrInvalidConfig)
	}
	dialAddress = net.JoinHostPort(address.String(), portText)
	return "http://" + dialAddress, dialAddress, nil
}

func sameAddress(left, right string) bool {
	leftHost, leftPort, leftErr := net.SplitHostPort(left)
	rightHost, rightPort, rightErr := net.SplitHostPort(right)
	if leftErr != nil || rightErr != nil || leftPort != rightPort {
		return false
	}
	leftIP, leftErr := netip.ParseAddr(leftHost)
	rightIP, rightErr := netip.ParseAddr(rightHost)
	return leftErr == nil && rightErr == nil && leftIP == rightIP
}

func isControl(r rune) bool { return r < 0x20 || r == 0x7f }
