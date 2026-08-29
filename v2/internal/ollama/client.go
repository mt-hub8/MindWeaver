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
	"net/http/httptrace"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

const (
	MaxPromptBytes         = 64 << 10
	maxRequestBytes        = 128 << 10
	maxResponseBytes       = 4 << 20
	maxResponseHeaderBytes = 64 << 10
	maxModels              = 1000
	maxJSONNestingDepth    = 128
	maxJSONTokens          = 64 << 10
	maxJSONKeys            = 16 << 10
	maxJSONItems           = 16 << 10
	defaultTimeout         = 60 * time.Second
)

var (
	ErrInvalidConfig    = errors.New("ollama: invalid configuration")
	ErrInvalidRequest   = errors.New("ollama: invalid request")
	ErrRequestTooLarge  = errors.New("ollama: request exceeds size limit")
	ErrResponseTooLarge = errors.New("ollama: response exceeds size limit")
	ErrProtocol         = errors.New("ollama: invalid response protocol")
	ErrUnavailable      = errors.New("ollama: service unavailable")
	ErrOutcomeUncertain = errors.New("ollama: provider outcome uncertain")
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

type tagsResponse struct {
	Models []tagModel `json:"models"`
}

type tagModel struct {
	Name string `json:"name"`
}

type chatResponse struct {
	Message chatMessage `json:"message"`
	Done    bool        `json:"done"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (response *tagsResponse) UnmarshalJSON(data []byte) error {
	if err := rejectKnownFieldAliases(data, "models"); err != nil {
		return err
	}
	type wireTagsResponse tagsResponse
	return json.Unmarshal(data, (*wireTagsResponse)(response))
}

func (model *tagModel) UnmarshalJSON(data []byte) error {
	if err := rejectKnownFieldAliases(data, "name"); err != nil {
		return err
	}
	type wireTagModel tagModel
	return json.Unmarshal(data, (*wireTagModel)(model))
}

func (response *chatResponse) UnmarshalJSON(data []byte) error {
	if err := rejectKnownFieldAliases(data, "message", "done"); err != nil {
		return err
	}
	type wireChatResponse chatResponse
	return json.Unmarshal(data, (*wireChatResponse)(response))
}

func (message *chatMessage) UnmarshalJSON(data []byte) error {
	if err := rejectKnownFieldAliases(data, "role", "content"); err != nil {
		return err
	}
	type wireChatMessage chatMessage
	return json.Unmarshal(data, (*wireChatMessage)(message))
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
		Proxy:                  nil,
		DisableCompression:     true,
		ForceAttemptHTTP2:      false,
		MaxConnsPerHost:        4,
		MaxIdleConnsPerHost:    2,
		IdleConnTimeout:        30 * time.Second,
		MaxResponseHeaderBytes: maxResponseHeaderBytes,
		ResponseHeaderTimeout:  timeout,
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
	var response tagsResponse
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
	var response chatResponse
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
	var requestWriteStarted atomic.Bool
	request = request.WithContext(httptrace.WithClientTrace(request.Context(), &httptrace.ClientTrace{
		// WroteRequest runs after the transport attempted the request write. An
		// error can still mean a prefix reached Ollama, so either result makes a
		// later transport failure unsafe to describe as definitely not executed.
		WroteRequest: func(httptrace.WroteRequestInfo) { requestWriteStarted.Store(true) },
	}))
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Accept-Encoding", "identity")
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
		if requestWriteStarted.Load() {
			return fmt.Errorf("%w: response was not received", ErrOutcomeUncertain)
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
	if len(response.Header.Values("Content-Encoding")) != 0 {
		return fmt.Errorf("%w: encoded responses are not accepted", ErrProtocol)
	}
	if !validJSONContentType(response.Header) {
		return fmt.Errorf("%w: response is not application/json", ErrProtocol)
	}
	if response.ContentLength > maxResponseBytes {
		return ErrResponseTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return classifyResponseReadError(ctx, err)
	}
	if len(data) > maxResponseBytes {
		return ErrResponseTooLarge
	}
	if !utf8.Valid(data) {
		return fmt.Errorf("%w: response is not valid UTF-8", ErrProtocol)
	}
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return fmt.Errorf("%w: malformed JSON", ErrProtocol)
	}
	if err := json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("%w: malformed JSON", ErrProtocol)
	}
	return nil
}

func validJSONContentType(header http.Header) bool {
	values := header.Values("Content-Type")
	if len(values) != 1 {
		return false
	}
	mediaType, parameters, err := mime.ParseMediaType(values[0])
	if err != nil || mediaType != "application/json" {
		return false
	}
	for name, value := range parameters {
		if name != "charset" || !strings.EqualFold(value, "utf-8") {
			return false
		}
	}
	return true
}

func classifyResponseReadError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return context.DeadlineExceeded
	}
	// Headers for a successful response were already received. A truncated or
	// otherwise unreadable body cannot prove whether Ollama completed the call.
	return fmt.Errorf("%w: response body was incomplete", ErrOutcomeUncertain)
}

func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	budget := &jsonScanBudget{}
	if err := scanJSONValue(decoder, 0, budget); err != nil {
		return err
	}
	if _, err := budget.nextToken(decoder); !errors.Is(err, io.EOF) {
		return errors.New("JSON response has trailing data")
	}
	return nil
}

type jsonScanBudget struct {
	tokens int
	keys   int
	items  int
}

func (budget *jsonScanBudget) nextToken(decoder *json.Decoder) (json.Token, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	budget.tokens++
	if budget.tokens > maxJSONTokens {
		return nil, errors.New("JSON response exceeds token limit")
	}
	return token, nil
}

func (budget *jsonScanBudget) addKey() error {
	budget.keys++
	if budget.keys > maxJSONKeys {
		return errors.New("JSON response exceeds object key limit")
	}
	return nil
}

func (budget *jsonScanBudget) addItem() error {
	budget.items++
	if budget.items > maxJSONItems {
		return errors.New("JSON response exceeds array item limit")
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder, depth int, budget *jsonScanBudget) error {
	token, err := budget.nextToken(decoder)
	if err != nil {
		return err
	}
	delimiter, composite := token.(json.Delim)
	if !composite {
		return nil
	}
	if depth >= maxJSONNestingDepth {
		return errors.New("JSON response exceeds nesting limit")
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := budget.nextToken(decoder)
			if err != nil {
				return err
			}
			if err := budget.addKey(); err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("JSON object key is not text")
			}
			if _, duplicate := seen[key]; duplicate {
				return errors.New("JSON response contains a duplicate object key")
			}
			seen[key] = struct{}{}
			if err := scanJSONValue(decoder, depth+1, budget); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := budget.addItem(); err != nil {
				return err
			}
			if err := scanJSONValue(decoder, depth+1, budget); err != nil {
				return err
			}
		}
	default:
		return errors.New("invalid JSON delimiter")
	}
	closing, err := budget.nextToken(decoder)
	if err != nil {
		return err
	}
	if delimiter == '{' && closing != json.Delim('}') || delimiter == '[' && closing != json.Delim(']') {
		return errors.New("mismatched JSON delimiter")
	}
	return nil
}

func rejectKnownFieldAliases(data []byte, knownFields ...string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for field := range fields {
		for _, known := range knownFields {
			if field != known && strings.EqualFold(field, known) {
				return errors.New("JSON response contains a mis-cased known field")
			}
		}
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
