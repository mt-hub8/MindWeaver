package browserqualification

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
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
	qualificationTimeout = 2 * time.Minute
	cleanupTimeout       = 5 * time.Second
	maxWebDriverBytes    = 1 << 20
	maxTraceBytes        = 4 << 10
	managedProcessCount  = 3
)

type protocolBoundary struct {
	approval artifactApproval
	bundle   string
	harness  processHarness
	timeout  time.Duration
}

type processHarness interface {
	// Start owns cleanup on a non-nil error. A successful Start transfers a
	// bounded tree to protocolBoundary, which always calls Cleanup.
	Start(context.Context, *approvedArtifactSet) (managedProcessTree, error)
}

type managedProcessTree interface {
	DriverEndpoint() string
	MindWeaverEndpoint() string
	Identities() []processIdentity
	Cleanup(context.Context) cleanupReceipt
}

type processIdentity struct {
	Role      string
	PID       int
	ParentPID int
	SHA256    string
}

type cleanupReceipt struct {
	AllExited    bool
	ProcessCount int
}

func defaultProcessBoundary(approval Approval, options RunOptions) processBoundary {
	if approval.artifact == nil {
		return nil
	}
	return &protocolBoundary{approval: *approval.artifact, bundle: options.BundleRoot, timeout: qualificationTimeout}
}

func (boundary *protocolBoundary) Run(parent context.Context) (processEvidence, BlockerCode, error) {
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
	if boundary.harness == nil {
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

	timeout := boundary.timeout
	if timeout <= 0 || timeout > qualificationTimeout {
		timeout = qualificationTimeout
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	tree, err := boundary.harness.Start(ctx, artifacts)
	if err != nil || tree == nil {
		return processEvidence{}, "", errors.New("controlled process start failed")
	}
	cleaned := false
	cleanup := func() cleanupReceipt {
		if cleaned {
			return cleanupReceipt{}
		}
		cleaned = true
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cleanupCancel()
		return tree.Cleanup(cleanupCtx)
	}
	defer func() {
		if !cleaned {
			_ = cleanup()
		}
	}()

	identities := tree.Identities()
	if !validManagedProcessTree(identities, boundary.approval) {
		_ = cleanup()
		return processEvidence{}, "", errors.New("controlled process identity failed")
	}
	driverEndpoint, err := literalLoopbackEndpoint(tree.DriverEndpoint())
	if err != nil {
		_ = cleanup()
		return processEvidence{}, "", errors.New("controlled driver endpoint failed")
	}
	mindweaverEndpoint, err := literalLoopbackEndpoint(tree.MindWeaverEndpoint())
	if err != nil {
		_ = cleanup()
		return processEvidence{}, "", errors.New("controlled application endpoint failed")
	}
	client := newWebDriverClient(driverEndpoint)
	sessionID, err := client.createSession(ctx)
	if err != nil {
		_ = cleanup()
		return processEvidence{}, "", errors.New("controlled webdriver session failed")
	}
	sessionClosed := false
	defer func() {
		if !sessionClosed {
			closeCtx, closeCancel := context.WithTimeout(context.Background(), cleanupTimeout)
			defer closeCancel()
			_ = client.deleteSession(closeCtx, sessionID)
		}
	}()

	scenarios := make([]ScenarioResult, 0, len(requiredScenarioIDs))
	for _, id := range requiredScenarioIDs {
		result, err := client.runControlledScenario(ctx, sessionID, mindweaverEndpoint, id)
		if err != nil {
			closeCtx, closeCancel := context.WithTimeout(context.Background(), cleanupTimeout)
			_ = client.deleteSession(closeCtx, sessionID)
			closeCancel()
			sessionClosed = true
			_ = cleanup()
			return processEvidence{}, "", errors.New("controlled webdriver scenario failed")
		}
		scenarios = append(scenarios, result)
	}
	if err := client.deleteSession(ctx, sessionID); err != nil {
		sessionClosed = true
		_ = cleanup()
		return processEvidence{}, "", errors.New("controlled webdriver cleanup failed")
	}
	sessionClosed = true
	if artifacts.Reverify() != nil {
		_ = cleanup()
		return processEvidence{}, "", errors.New("artifact identity changed")
	}
	receipt := cleanup()
	if !receipt.AllExited || receipt.ProcessCount != managedProcessCount {
		return processEvidence{}, "", errors.New("controlled process cleanup failed")
	}
	if artifacts.Reverify() != nil {
		return processEvidence{}, "", errors.New("artifact identity changed")
	}
	if err := artifacts.Close(); err != nil {
		return processEvidence{}, "", errors.New("artifact close failed")
	}
	closed = true
	cleanupDigest := cleanupReceiptDigest(boundary.approval)
	return processEvidence{
		ApprovalID: boundary.approval.ID, BrowserSHA256: boundary.approval.Browser.SHA256,
		DriverSHA256: boundary.approval.Driver.SHA256, ExecutableSHA256: boundary.approval.MindWeaver.SHA256,
		BrowserVersion: boundary.approval.Browser.Version, DriverVersion: boundary.approval.Driver.Version,
		Scenarios: scenarios, CleanupStatus: "HARNESS_PASS", CleanupSHA256: fmt.Sprintf("%x", cleanupDigest[:]),
		CleanupProcesses: receipt.ProcessCount,
	}, BlockerControlledHarness, nil
}

func cleanupReceiptDigest(approval artifactApproval) [sha256.Size]byte {
	canonical := "mindweaver-browser-cleanup-v1\n" +
		"approval=" + approval.ID + "\n" +
		"browser=" + approval.Browser.SHA256 + "\n" +
		"driver=" + approval.Driver.SHA256 + "\n" +
		"mindweaver=" + approval.MindWeaver.SHA256 + "\n" +
		"all_exited=true\nprocess_count=3\nsession_closed=true\nartifacts_reverified=true\n"
	return sha256.Sum256([]byte(canonical))
}

func validManagedProcessTree(identities []processIdentity, approval artifactApproval) bool {
	if len(identities) != managedProcessCount {
		return false
	}
	byRole := make(map[string]processIdentity, len(identities))
	byPID := make(map[int]struct{}, len(identities))
	for _, identity := range identities {
		if identity.PID <= 0 || !lowerSHA256(identity.SHA256) {
			return false
		}
		if _, duplicate := byRole[identity.Role]; duplicate {
			return false
		}
		if _, duplicate := byPID[identity.PID]; duplicate {
			return false
		}
		byRole[identity.Role] = identity
		byPID[identity.PID] = struct{}{}
	}
	browser, browserOK := byRole["browser"]
	driver, driverOK := byRole["driver"]
	mindweaver, mindweaverOK := byRole["mindweaver"]
	return browserOK && driverOK && mindweaverOK && browser.SHA256 == approval.Browser.SHA256 &&
		driver.SHA256 == approval.Driver.SHA256 && mindweaver.SHA256 == approval.MindWeaver.SHA256 &&
		browser.ParentPID == driver.PID && driver.ParentPID == 0 && mindweaver.ParentPID == 0
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

func (client *webDriverClient) runControlledScenario(ctx context.Context, sessionID string, target *url.URL, scenarioID string) (ScenarioResult, error) {
	targetCopy := *target
	targetCopy.Path = "/__browser_qualification/" + url.PathEscape(strings.ToLower(scenarioID))
	payload, err := json.Marshal(map[string]string{"url": targetCopy.String()})
	if err != nil {
		return ScenarioResult{}, errors.New("webdriver request rejected")
	}
	base := "/session/" + url.PathEscape(sessionID)
	if _, err := client.request(ctx, http.MethodPost, base+"/url", payload); err != nil {
		return ScenarioResult{}, err
	}
	response, err := client.request(ctx, http.MethodGet, base+"/screenshot", nil)
	if err != nil {
		return ScenarioResult{}, err
	}
	var screenshotWire struct {
		Value string `json:"value"`
	}
	if json.Unmarshal(response, &screenshotWire) != nil || len(screenshotWire.Value) > base64.StdEncoding.EncodedLen(maxWebDriverBytes) {
		return ScenarioResult{}, errors.New("webdriver screenshot rejected")
	}
	screenshot, err := base64.StdEncoding.DecodeString(screenshotWire.Value)
	if err != nil || len(screenshot) == 0 || len(screenshot) > maxWebDriverBytes {
		return ScenarioResult{}, errors.New("webdriver screenshot rejected")
	}
	trace := []byte("mindweaver-controlled-webdriver-trace-v1\nscenario=" + scenarioID + "\nnavigation=ok\nscreenshot=ok\n")
	screenshotDigest := sha256.Sum256(screenshot)
	traceDigest := sha256.Sum256(trace)
	return ScenarioResult{
		ID: scenarioID, Status: "HARNESS_PASS", Code: "NOT_QUALIFIED",
		ScreenshotSHA256: fmt.Sprintf("%x", screenshotDigest[:]), ScreenshotBytes: int64(len(screenshot)),
		TraceSHA256: fmt.Sprintf("%x", traceDigest[:]), TraceBytes: int64(len(trace)),
	}, nil
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
