package browserqualification

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	cleanupTimeout    = 5 * time.Second
	maxWebDriverBytes = 1 << 20
	maxTraceBytes     = 4 << 10
)

type protocolBoundary struct {
	approval artifactApproval
	bundle   string
}

type processIdentity struct {
	Role      string
	PID       int
	ParentPID int
	SHA256    string
}

type cleanupReceipt struct {
	AllExited       bool
	ProcessCount    int
	ActiveProcesses int
}

func defaultProcessBoundary(approval Approval, options RunOptions) processBoundary {
	if approval.artifact == nil {
		return nil
	}
	return &protocolBoundary{approval: *approval.artifact, bundle: options.BundleRoot}
}

func (boundary *protocolBoundary) Run(_ context.Context) (processEvidence, BlockerCode, error) {
	artifacts, err := openApprovedArtifacts(boundary.approval, boundary.bundle)
	if err != nil {
		return processEvidence{}, BlockerArtifactBundleInvalid, nil
	}
	closed := false
	defer func() {
		if !closed {
			_ = artifacts.Close()
		}
	}()
	if artifacts.Reverify() != nil {
		return processEvidence{}, BlockerArtifactBundleInvalid, nil
	}
	blocker := BlockerProcessSandboxUnavailable
	if processSandboxAvailable() {
		blocker = BlockerLaunchProfileNotApproved
	}
	if err := artifacts.Close(); err != nil {
		return processEvidence{}, "", errors.New("artifact close failed")
	}
	closed = true
	return processEvidence{}, blocker, nil
}

func literalLoopbackEndpoint(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return nil, errors.New("invalid loopback endpoint")
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port < 1 || port > 65535 || parsed.Host != net.JoinHostPort("127.0.0.1", strconv.Itoa(port)) {
		return nil, errors.New("invalid loopback endpoint")
	}
	parsed.Path = ""
	return parsed, nil
}

type webDriverClient struct {
	endpoint *url.URL
	client   *http.Client
}

func newWebDriverClient(endpoint *url.URL) *webDriverClient {
	address := endpoint.Host
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			if network != "tcp" && network != "tcp4" {
				return nil, errors.New("webdriver network rejected")
			}
			return (&net.Dialer{}).DialContext(ctx, "tcp4", address)
		},
		DisableKeepAlives: true,
	}
	return &webDriverClient{endpoint: endpoint, client: &http.Client{
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("webdriver redirect rejected") },
	}}
}

func (client *webDriverClient) createSession(ctx context.Context) (string, error) {
	response, err := client.request(ctx, http.MethodPost, "/session", []byte(`{"capabilities":{"alwaysMatch":{}}}`))
	if err != nil {
		return "", err
	}
	var wire struct {
		Value struct {
			SessionID string `json:"sessionId"`
		} `json:"value"`
		SessionID string `json:"sessionId"`
	}
	if json.Unmarshal(response, &wire) != nil {
		return "", errors.New("webdriver response rejected")
	}
	sessionID := wire.Value.SessionID
	if sessionID == "" {
		sessionID = wire.SessionID
	}
	if !stableToken(sessionID, 64) {
		return "", errors.New("webdriver response rejected")
	}
	return sessionID, nil
}

func (client *webDriverClient) deleteSession(ctx context.Context, sessionID string) error {
	_, err := client.request(ctx, http.MethodDelete, "/session/"+url.PathEscape(sessionID), nil)
	return err
}

func (client *webDriverClient) request(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	if len(body) > maxWebDriverBytes || !strings.HasPrefix(path, "/") {
		return nil, errors.New("webdriver request rejected")
	}
	request, err := http.NewRequestWithContext(ctx, method, client.endpoint.String()+path, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("webdriver request rejected")
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.client.Do(request)
	if err != nil {
		return nil, errors.New("webdriver request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, errors.New("webdriver response rejected")
	}
	mediaType, _, mediaErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if mediaErr != nil || mediaType != "application/json" {
		return nil, errors.New("webdriver response rejected")
	}
	limited := io.LimitReader(response.Body, maxWebDriverBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil || len(data) > maxWebDriverBytes {
		return nil, errors.New("webdriver response rejected")
	}
	return data, nil
}
