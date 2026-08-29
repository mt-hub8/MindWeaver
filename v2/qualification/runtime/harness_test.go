package runtimequalification_test

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	repetitionEnvironment = "MW_RUNTIME_QUALIFICATION_COUNT"
	reportEnvironment     = "MW_RUNTIME_QUALIFICATION_REPORT"
	pdfCoordinatorEnv     = "MWQ_RUNTIME_PDF_COORDINATOR"
	maxHTTPBody           = 5 << 20
	processReapTimeout    = 10 * time.Second
)

type qualificationProtocol struct {
	SchemaVersion      int            `json:"schema_version"`
	DefaultRepetitions int            `json:"default_repetitions"`
	MaximumRepetitions int            `json:"maximum_repetitions"`
	Cases              []protocolCase `json:"cases"`
}

type protocolCase struct {
	CaseCode       string `json:"case_code"`
	CheckpointCode string `json:"checkpoint_code"`
	FinalCode      string `json:"final_code"`
}

type builtArtifacts struct {
	mindweaver string
	pdfHelper  string
	sha256     string
	pdfSHA256  string
	goVersion  string
}

func loadQualificationProtocol(t *testing.T, root string) qualificationProtocol {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "testdata", "qualification", "runtime", "protocol-v1.json"))
	if err != nil {
		t.Fatal("read runtime qualification protocol")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var protocol qualificationProtocol
	if err := decoder.Decode(&protocol); err != nil {
		t.Fatal("decode runtime qualification protocol")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		t.Fatal("runtime qualification protocol contains trailing data")
	}
	if protocol.SchemaVersion != 1 || protocol.DefaultRepetitions < 1 ||
		protocol.MaximumRepetitions < protocol.DefaultRepetitions || len(protocol.Cases) != 2 {
		t.Fatal("runtime qualification protocol is outside the frozen v1 bounds")
	}
	want := []protocolCase{
		{CaseCode: "ANSWER_PROVIDER_RECEIVED_BEFORE_TERMINAL", CheckpointCode: "PROVIDER_REQUEST_RECEIVED_ANSWER_PENDING", FinalCode: "ANSWER_FAILED_OUTCOME_UNCERTAIN"},
		{CaseCode: "INGESTION_ACCEPTED_WHILE_WORKER_OCCUPIED", CheckpointCode: "BLOCKER_RUNNING_TARGET_QUEUED", FinalCode: "TARGET_SUCCEEDED_SEARCHABLE_NO_DUPLICATES"},
	}
	for index := range want {
		if protocol.Cases[index] != want[index] {
			t.Fatal("runtime qualification protocol case drifted")
		}
	}
	return protocol
}

func qualificationRepetitions(t *testing.T, protocol qualificationProtocol) int {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv(repetitionEnvironment))
	if raw == "" {
		return protocol.DefaultRepetitions
	}
	count, err := strconv.Atoi(raw)
	if err != nil || count < 1 || count > protocol.MaximumRepetitions {
		t.Fatalf("%s must be between 1 and %d", repetitionEnvironment, protocol.MaximumRepetitions)
	}
	return count
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	current, err := os.Getwd()
	if err != nil {
		t.Fatal("resolve qualification working directory")
	}
	for {
		if info, statErr := os.Stat(filepath.Join(current, "go.mod")); statErr == nil && info.Mode().IsRegular() {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			t.Fatal("resolve v2 module root")
		}
		current = parent
	}
}

func frozenGoTool(t *testing.T) string {
	t.Helper()
	candidate := strings.TrimSpace(os.Getenv("MW_GO"))
	if candidate == "" {
		candidate = filepath.Join(runtime.GOROOT(), "bin", "go.exe")
	}
	absolute, err := filepath.Abs(candidate)
	if err != nil {
		t.Fatal("resolve Go tool")
	}
	info, err := os.Stat(absolute)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatal("frozen Go tool is unavailable")
	}
	return absolute
}

func hermeticEnvironment(extra map[string]string) []string {
	overrides := map[string]string{
		"CGO_ENABLED": "0", "GOARCH": "amd64", "GOAMD64": "v1", "GOOS": "windows",
		"GOENV": "off", "GOEXPERIMENT": "", "GOFIPS140": "off", "GOFLAGS": "-mod=readonly -trimpath -buildvcs=false",
		"GOTOOLCHAIN": "local", "GOTELEMETRY": "off", "GOWORK": "off",
	}
	for key, value := range extra {
		overrides[key] = value
	}
	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if ok {
			if _, replaced := overrides[strings.ToUpper(key)]; replaced {
				continue
			}
		}
		environment = append(environment, entry)
	}
	for key, value := range overrides {
		environment = append(environment, key+"="+value)
	}
	return environment
}

func buildQualificationArtifacts(t *testing.T, root string) builtArtifacts {
	t.Helper()
	goTool := frozenGoTool(t)
	versionCommand := exec.Command(goTool, "version")
	versionCommand.Env = hermeticEnvironment(nil)
	versionOutput, err := versionCommand.Output()
	if err != nil {
		t.Fatal("read frozen Go version")
	}
	goVersion := strings.TrimSpace(string(versionOutput))
	if !strings.Contains(goVersion, "go1.27.0") || !strings.Contains(goVersion, "windows/amd64") {
		t.Fatalf("qualification requires Go 1.27.0 windows/amd64, got %q", goVersion)
	}
	directory := t.TempDir()
	mindweaver := filepath.Join(directory, "mindweaver.exe")
	pdfHelper := filepath.Join(directory, "mindweaver-pdf.exe")
	for _, build := range []struct {
		output string
		target string
	}{
		{output: mindweaver, target: "./cmd/mindweaver"},
		{output: pdfHelper, target: "./qualification/runtime/pdfblocker"},
	} {
		command := exec.Command(goTool, "build", "-trimpath", "-buildvcs=false", "-o", build.output, build.target)
		command.Dir = root
		command.Env = hermeticEnvironment(nil)
		if output, buildErr := command.CombinedOutput(); buildErr != nil {
			t.Fatalf("build runtime qualification artifact: %v (%d diagnostic bytes)", buildErr, len(output))
		}
	}
	mindweaverDigest := hashArtifact(t, mindweaver)
	pdfDigest := hashArtifact(t, pdfHelper)
	return builtArtifacts{
		mindweaver: mindweaver, pdfHelper: pdfHelper,
		sha256: mindweaverDigest, pdfSHA256: pdfDigest, goVersion: goVersion,
	}
}

func hashArtifact(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("hash runtime qualification artifact")
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

type launchInfo struct {
	origin    string
	bootstrap string
}

type runningApp struct {
	command              *exec.Cmd
	stderr               *boundedBuffer
	launch               launchInfo
	waitDone             <-chan error
	waitErr              error
	nativeHandle         uintptr
	nativeCloseAttempted bool
	nativeCloseErr       error
	finished             bool
	mu                   sync.Mutex
}

func startMindWeaver(t *testing.T, artifacts builtArtifacts, root, coordinator string) *runningApp {
	t.Helper()
	extra := map[string]string{}
	if coordinator != "" {
		extra[pdfCoordinatorEnv] = coordinator
	}
	return startMindWeaverWithEnvironment(t, artifacts, root, hermeticEnvironment(extra))
}

func startMindWeaverWithEnvironment(t *testing.T, artifacts builtArtifacts, root string, environment []string) *runningApp {
	t.Helper()
	configPath := filepath.Join(root, "mindweaver.v1.json")
	vaultPath := filepath.Join(root, "vault")
	command := exec.Command(artifacts.mindweaver, "serve", "-config", configPath, "-vault", vaultPath, "-no-browser")
	command.Dir = root
	command.Env = environment
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal("open MindWeaver stdout")
	}
	diagnostics := &boundedBuffer{limit: 16 << 10}
	command.Stderr = diagnostics
	if err := command.Start(); err != nil {
		t.Fatal("start real MindWeaver binary")
	}
	nativeHandle, retainErr := retainREL001ProcessHandle(command.Process.Pid)
	waitDone := make(chan error, 1)
	go func() { waitDone <- command.Wait() }()
	if retainErr != nil {
		t.Cleanup(func() {
			if command.Process.Kill() != nil {
				t.Error("retain process handle cleanup kill failed")
			}
			select {
			case <-waitDone:
			case <-time.After(processReapTimeout):
				t.Error("retain process handle cleanup timed out")
			}
		})
		t.Fatal("retain process handle failed")
	}
	app := &runningApp{command: command, stderr: diagnostics, waitDone: waitDone, nativeHandle: nativeHandle}
	t.Cleanup(func() {
		if err := app.stop(); err != nil {
			t.Error("MindWeaver process cleanup failed")
		}
	})
	launched := make(chan launchInfo, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 4096), 64<<10)
		for scanner.Scan() {
			if launch, ok := parseLaunchLine(scanner.Text()); ok {
				select {
				case launched <- launch:
				default:
				}
			}
		}
	}()
	select {
	case app.launch = <-launched:
		return app
	case <-time.After(20 * time.Second):
		if err := app.stop(); err != nil {
			t.Fatal("MindWeaver readiness cleanup failed")
		}
		t.Fatalf("MindWeaver did not expose its ready checkpoint (%d diagnostic bytes)", diagnostics.Len())
		return nil
	}
}

func parseLaunchLine(line string) (launchInfo, bool) {
	parsed, err := url.Parse(strings.TrimSpace(line))
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" || parsed.Port() == "" ||
		parsed.Path != "/" || parsed.RawQuery != "" || !strings.HasPrefix(parsed.Fragment, "bootstrap=") {
		return launchInfo{}, false
	}
	token := strings.TrimPrefix(parsed.Fragment, "bootstrap=")
	if token == "" || strings.Contains(token, "&") {
		return launchInfo{}, false
	}
	return launchInfo{origin: "http://" + parsed.Host, bootstrap: token}, true
}

func (app *runningApp) stop() error {
	return stopREL001Process(app, false)
}

func (app *runningApp) terminate(t *testing.T) {
	t.Helper()
	if err := stopREL001Process(app, true); err != nil {
		t.Fatal("controlled termination did not reap a killed process")
	}
}

// awaitWaitLocked is the sole consumer of the Cmd.Wait result. Callers must
// hold app.mu. A timeout leaves the result channel available for one later,
// bounded cleanup attempt and never marks the process as finished.
func (app *runningApp) awaitWaitLocked(timeout time.Duration) (error, bool) {
	if app.finished {
		return app.waitErr, true
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-app.waitDone:
		app.waitErr = err
		app.finished = true
		return err, true
	case <-timer.C:
		return nil, false
	}
}

type boundedBuffer struct {
	mu       sync.Mutex
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

func (buffer *boundedBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	remaining := buffer.limit - buffer.buffer.Len()
	if remaining > 0 {
		_, _ = buffer.buffer.Write(data[:min(len(data), remaining)])
	}
	if len(data) > remaining {
		buffer.overflow = true
	}
	return len(data), nil
}

func (buffer *boundedBuffer) Len() int {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buffer.Len()
}

type apiSession struct {
	origin string
	csrf   string
	client *http.Client
}

func exchangeSession(t *testing.T, app *runningApp) *apiSession {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal("create loopback cookie jar")
	}
	transport := &http.Transport{Proxy: nil, DisableCompression: true}
	client := &http.Client{Transport: transport, Jar: jar, Timeout: 30 * time.Second}
	t.Cleanup(transport.CloseIdleConnections)
	request, err := http.NewRequest(http.MethodPost, app.launch.origin+"/bootstrap/exchange", nil)
	if err != nil {
		t.Fatal("create bootstrap exchange")
	}
	request.Header.Set("Origin", app.launch.origin)
	request.Header.Set("X-MindWeaver-Bootstrap", app.launch.bootstrap)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal("exchange bootstrap")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("bootstrap exchange status = %d", response.StatusCode)
	}
	var wire struct {
		CSRF string `json:"csrfToken"`
	}
	decodeResponse(t, response.Body, &wire)
	if wire.CSRF == "" {
		t.Fatal("bootstrap exchange returned no CSRF token")
	}
	app.launch.bootstrap = ""
	return &apiSession{origin: app.launch.origin, csrf: wire.CSRF, client: client}
}

func (session *apiSession) request(t *testing.T, method, path string, body io.Reader, contentType string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, session.origin+path, body)
	if err != nil {
		t.Fatal("create loopback API request")
	}
	request.Header.Set("Origin", session.origin)
	if method != http.MethodGet && method != http.MethodHead {
		request.Header.Set("X-MindWeaver-CSRF", session.csrf)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response, err := session.client.Do(request)
	if err != nil {
		t.Fatal("execute loopback API request")
	}
	return response
}

func (session *apiSession) json(t *testing.T, method, path string, input, output any, accepted ...int) int {
	t.Helper()
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			t.Fatal("encode loopback request")
		}
		body = bytes.NewReader(encoded)
	}
	response := session.request(t, method, path, body, "application/json")
	defer response.Body.Close()
	if !containsStatus(accepted, response.StatusCode) {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxHTTPBody))
		t.Fatalf("loopback API %s %s status = %d", method, pathCode(path), response.StatusCode)
	}
	if output != nil {
		decodeResponse(t, response.Body, output)
	} else {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxHTTPBody))
	}
	return response.StatusCode
}

func decodeResponse(t *testing.T, reader io.Reader, output any) {
	t.Helper()
	decoder := json.NewDecoder(io.LimitReader(reader, maxHTTPBody+1))
	if err := decoder.Decode(output); err != nil {
		t.Fatal("decode bounded loopback response")
	}
}

func containsStatus(statuses []int, got int) bool {
	for _, status := range statuses {
		if status == got {
			return true
		}
	}
	return false
}

func pathCode(path string) string {
	path, _, _ = strings.Cut(path, "?")
	return path
}

type uploadResult struct {
	DocumentID string `json:"documentId"`
	RevisionID string `json:"revisionId"`
	JobID      string `json:"jobId"`
	Created    bool   `json:"created"`
}

func upload(t *testing.T, session *apiSession, key, title, filename string, content []byte) uploadResult {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, session.origin+"/api/v1/documents/upload", bytes.NewReader(content))
	if err != nil {
		t.Fatal("create upload request")
	}
	request.Header.Set("Origin", session.origin)
	request.Header.Set("X-MindWeaver-CSRF", session.csrf)
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("Idempotency-Key", key)
	request.Header.Set("X-MindWeaver-Title-B64", encodedHeader(title))
	request.Header.Set("X-MindWeaver-Filename-B64", encodedHeader(filename))
	response, err := session.client.Do(request)
	if err != nil {
		t.Fatal("execute upload request")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted && response.StatusCode != http.StatusOK {
		t.Fatalf("upload status = %d", response.StatusCode)
	}
	var result uploadResult
	decodeResponse(t, response.Body, &result)
	if result.DocumentID == "" || result.RevisionID == "" || result.JobID == "" {
		t.Fatal("upload response omitted durable identifiers")
	}
	return result
}

func hashIdentifier(raw string) string {
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:])
}

func encodedHeader(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}
