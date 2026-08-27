package runtimequalification_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	rel001KillCampaignEnvironment = "MW_REL001_HTTP_KILL_CAMPAIGN"
	rel001KillCampaignOptIn       = "RUN_1024_V1"
	rel001KillCampaignRuns        = 1024
	rel001KillSmokeRuns           = 4
	rel001KillMaximumDelay        = 100 * time.Millisecond
	rel001KillRequestTimeout      = 10 * time.Second
	rel001KillMaximumBody         = 1 << 20
	rel001KillPlanSHA256          = "454e42d3fc6ac6eaf834a64ac0d5a736ab97f06e81e3c43128f68874b035863d"
	rel001KillSeedHex             = "72189cd5506845ea8bdd10c3849cd2a1c6391e90ab5f32608b33fc7d1826e8a4"
)

type rel001KillMutation byte

const (
	rel001KillCollection rel001KillMutation = iota
	rel001KillConversation
	rel001KillUpload
	rel001KillAskNoContext
)

var rel001KillMutationNames = [...]string{
	"collection-create", "conversation-create", "txt-upload", "ask-no-context",
}

type rel001KillFrame struct {
	sequence int
	mutation rel001KillMutation
	delay    time.Duration
}

type rel001KillTemplate struct {
	method, path, key, contentType  string
	expected, title, filename, body string
}

type rel001KillIdentity struct {
	primary, secondary, tertiary string
	status, limitation, code     string
}

type rel001KillResponse struct {
	identity rel001KillIdentity
	err      error
}

func TestREL001DeterministicHTTPKillReplay(t *testing.T) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		t.Skip("REL-001 process-kill qualification requires Windows amd64")
	}
	enabled, err := selectREL001KillCampaign(os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	frames := rel001KillFrames(t)
	limit := rel001KillSmokeRuns
	if enabled {
		limit = len(frames)
	}
	root := moduleRoot(t)
	artifacts := buildREL001KillBinary(t, root)
	campaignRoot := t.TempDir()
	linearizations := [4][2]int{}
	for _, frame := range frames[:limit] {
		frame := frame
		t.Run(fmt.Sprintf("%04d-%s", frame.sequence, rel001KillMutationNames[frame.mutation]), func(t *testing.T) {
			acknowledged := runREL001KillFrame(t, artifacts, campaignRoot, frame)
			index := 1
			if acknowledged {
				index = 0
			}
			linearizations[frame.mutation][index]++
		})
	}
	if entries, err := os.ReadDir(campaignRoot); err != nil || len(entries) != 0 {
		t.Fatal("REL001_KILL_CAMPAIGN_CLEANUP_INCOMPLETE")
	}
	if enabled {
		total := 0
		for mutation, counts := range linearizations {
			total += counts[0] + counts[1]
			if counts[0]+counts[1] != rel001KillCampaignRuns/len(linearizations) {
				t.Fatalf("REL001_KILL_MUTATION_COUNT_DRIFT mutation=%s total=%d",
					rel001KillMutationNames[mutation], counts[0]+counts[1])
			}
			if counts[0] == 0 || counts[1] == 0 {
				t.Fatalf("REL001_KILL_LINEARIZATION_MISSING mutation=%s acknowledged=%d killed=%d",
					rel001KillMutationNames[mutation], counts[0], counts[1])
			}
		}
		if total != rel001KillCampaignRuns {
			t.Fatalf("REL001_KILL_CAMPAIGN_COUNT_DRIFT total=%d", total)
		}
	}
}

func TestREL001KillPlanAndChildEnvironmentAreFailClosed(t *testing.T) {
	frames := rel001KillFrames(t)
	if len(frames) != rel001KillCampaignRuns {
		t.Fatal("REL001_KILL_PLAN_LENGTH_DRIFT")
	}
	counts := [4]int{}
	for sequence, frame := range frames {
		if frame.sequence != sequence || frame.mutation != rel001KillMutation(sequence%4) ||
			frame.delay < 0 || frame.delay > rel001KillMaximumDelay {
			t.Fatal("REL001_KILL_PLAN_FRAME_INVALID")
		}
		counts[frame.mutation]++
	}
	for _, count := range counts {
		if count != rel001KillCampaignRuns/4 {
			t.Fatal("REL001_KILL_PLAN_DISTRIBUTION_DRIFT")
		}
	}
	if got := rel001KillPlanDigest(frames); got != rel001KillPlanSHA256 {
		t.Fatalf("REL001_KILL_PLAN_DIGEST_DRIFT got=%s", got)
	}

	for _, test := range []struct {
		name        string
		environment []string
		want        bool
		wantError   bool
	}{
		{name: "absent"},
		{name: "exact", environment: []string{rel001KillCampaignEnvironment + "=" + rel001KillCampaignOptIn}, want: true},
		{name: "wrong value", environment: []string{rel001KillCampaignEnvironment + "=RUN"}, wantError: true},
		{name: "unknown", environment: []string{"MW_REL001_HTTP_KILL_EXTRA=1"}, wantError: true},
		{name: "duplicate", environment: []string{
			rel001KillCampaignEnvironment + "=" + rel001KillCampaignOptIn,
			strings.ToLower(rel001KillCampaignEnvironment) + "=" + rel001KillCampaignOptIn,
		}, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := selectREL001KillCampaign(test.environment)
			if got != test.want || (err != nil) != test.wantError {
				t.Fatalf("selection=%t error=%v", got, err)
			}
		})
	}

	temporaryDirectory := filepath.Join(t.TempDir(), "child")
	environment, err := rel001KillChildEnvironment([]string{
		"SystemRoot=C:\\Windows",
		"WINDIR=C:\\Windows",
		"Path=preserved",
		"MwQ_Run002_Child_Mode=attack",
		"mw_rel001_http_kill_campaign=" + rel001KillCampaignOptIn,
		"MINDWEAVER_SERVE_HELPER=1",
		"HTTPS_PROXY=http://hostile.invalid",
		"ALL_PROXY=http://hostile.invalid",
	}, temporaryDirectory)
	if err != nil {
		t.Fatal(err)
	}
	values := make(map[string][]string)
	for _, entry := range environment {
		name, value, ok := strings.Cut(entry, "=")
		if !ok {
			t.Fatal("REL001_KILL_CHILD_ENVIRONMENT_MALFORMED")
		}
		values[strings.ToUpper(name)] = append(values[strings.ToUpper(name)], value)
	}
	if len(values["MWQ_RUN002_CHILD_MODE"]) != 0 || len(values[rel001KillCampaignEnvironment]) != 0 ||
		len(values["MINDWEAVER_SERVE_HELPER"]) != 0 || len(values["HTTPS_PROXY"]) != 0 ||
		len(values["ALL_PROXY"]) != 0 || len(values["PATH"]) != 0 ||
		len(values["SYSTEMROOT"]) != 1 || len(values["WINDIR"]) != 1 ||
		len(values["TEMP"]) != 1 || values["TEMP"][0] != temporaryDirectory ||
		len(values["TMP"]) != 1 || values["TMP"][0] != temporaryDirectory ||
		len(values) != 4 {
		t.Fatal("REL001_KILL_CHILD_ENVIRONMENT_NOT_SANITIZED")
	}
	if _, err := rel001KillChildEnvironment([]string{"SystemRoot=C:\\Windows", "systemroot=C:\\Other"}, `C:\temp`); err == nil {
		t.Fatal("REL001_KILL_CHILD_ENVIRONMENT_DUPLICATE_ACCEPTED")
	}
}

func selectREL001KillCampaign(environment []string) (bool, error) {
	seen := false
	value := ""
	for _, entry := range environment {
		name, candidate, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		upper := strings.ToUpper(name)
		if !strings.HasPrefix(upper, "MW_REL001_HTTP_KILL_") {
			continue
		}
		if upper != rel001KillCampaignEnvironment || seen {
			return false, errors.New("REL001_KILL_CAMPAIGN_ENVIRONMENT_INVALID")
		}
		seen = true
		value = candidate
	}
	if !seen {
		return false, nil
	}
	if value != rel001KillCampaignOptIn {
		return false, errors.New("REL001_KILL_CAMPAIGN_OPT_IN_INVALID")
	}
	return true, nil
}

func rel001KillFrames(t *testing.T) []rel001KillFrame {
	t.Helper()
	seed, err := hex.DecodeString(rel001KillSeedHex)
	if err != nil || len(seed) != sha256.Size {
		t.Fatal("REL001_KILL_SEED_INVALID")
	}
	frames := make([]rel001KillFrame, rel001KillCampaignRuns)
	var encoded [8]byte
	for sequence := range frames {
		binary.BigEndian.PutUint64(encoded[:], uint64(sequence))
		input := append(append(append([]byte(nil), seed...), encoded[:]...), []byte("delay-v1")...)
		digest := sha256.Sum256(input)
		delay := binary.BigEndian.Uint64(digest[:8]) % uint64(rel001KillMaximumDelay/time.Microsecond+1)
		frames[sequence] = rel001KillFrame{
			sequence: sequence, mutation: rel001KillMutation(sequence % 4), delay: time.Duration(delay) * time.Microsecond,
		}
	}
	return frames
}

func rel001KillPlanDigest(frames []rel001KillFrame) string {
	digest := sha256.New()
	_, _ = digest.Write([]byte("mindweaver.rel001.http-kill-plan/v1\x00"))
	var encoded [9]byte
	for _, frame := range frames {
		binary.BigEndian.PutUint32(encoded[:4], uint32(frame.sequence))
		encoded[4] = byte(frame.mutation)
		binary.BigEndian.PutUint32(encoded[5:], uint32(frame.delay/time.Microsecond))
		_, _ = digest.Write(encoded[:])
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func rel001KillChildEnvironment(parent []string, temporaryDirectory string) ([]string, error) {
	if !filepath.IsAbs(temporaryDirectory) || filepath.Clean(temporaryDirectory) != temporaryDirectory {
		return nil, errors.New("REL001_KILL_CHILD_TEMPORARY_DIRECTORY_INVALID")
	}
	required := map[string]string{"SYSTEMROOT": "", "WINDIR": ""}
	seen := map[string]bool{}
	for _, entry := range parent {
		name, value, ok := strings.Cut(entry, "=")
		upper := strings.ToUpper(name)
		if !ok || upper != "SYSTEMROOT" && upper != "WINDIR" {
			continue
		}
		if seen[upper] || strings.TrimSpace(value) == "" {
			return nil, errors.New("REL001_KILL_CHILD_PARENT_ENVIRONMENT_INVALID")
		}
		seen[upper] = true
		required[upper] = value
	}
	if !seen["SYSTEMROOT"] || !seen["WINDIR"] {
		return nil, errors.New("REL001_KILL_CHILD_PARENT_ENVIRONMENT_INCOMPLETE")
	}
	return []string{
		"SystemRoot=" + required["SYSTEMROOT"],
		"WINDIR=" + required["WINDIR"],
		"TEMP=" + temporaryDirectory,
		"TMP=" + temporaryDirectory,
	}, nil
}

func buildREL001KillBinary(t *testing.T, root string) builtArtifacts {
	t.Helper()
	goTool := frozenGoTool(t)
	moduleCache, buildCache, buildTemporary := t.TempDir(), t.TempDir(), t.TempDir()
	buildEnvironment, err := rel001KillChildEnvironment(os.Environ(), buildTemporary)
	if err != nil {
		t.Fatal(err)
	}
	buildEnvironment = append(buildEnvironment,
		"CGO_ENABLED=0", "GOARCH=amd64", "GOAMD64=v1", "GOOS=windows",
		"GOENV=off", "GOEXPERIMENT=", "GOFIPS140=off", "GOFLAGS=-mod=vendor -trimpath -buildvcs=false",
		"GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local", "GOTELEMETRY=off", "GOVCS=*:off", "GOWORK=off",
		"GOMODCACHE="+moduleCache, "GOCACHE="+buildCache, "GOTMPDIR="+buildTemporary,
	)
	command := exec.Command(goTool, "version")
	command.Env = buildEnvironment
	output, err := command.Output()
	if err != nil || strings.TrimSpace(string(output)) != "go version go1.27.0 windows/amd64" {
		t.Fatal("REL001_KILL_TOOLCHAIN_INVALID")
	}
	binaryPath := filepath.Join(t.TempDir(), "mindweaver.exe")
	command = exec.Command(goTool, "build", "-trimpath", "-buildvcs=false", "-o", binaryPath, "./cmd/mindweaver")
	command.Dir = root
	command.Env = buildEnvironment
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("REL001_KILL_BUILD_FAILED bytes=%d", len(output))
	}
	if entries, err := os.ReadDir(moduleCache); err != nil || len(entries) != 0 {
		t.Fatal("REL001_KILL_MODULE_CACHE_NOT_EMPTY")
	}
	if info, err := os.Stat(filepath.Join(filepath.Dir(binaryPath), "mindweaver-pdf.exe")); err == nil || !errors.Is(err, os.ErrNotExist) || info != nil {
		t.Fatal("REL001_KILL_UNEXPECTED_HELPER_ARTIFACT")
	}
	return builtArtifacts{mindweaver: binaryPath}
}

func runREL001KillFrame(t *testing.T, artifacts builtArtifacts, campaignRoot string, frame rel001KillFrame) bool {
	t.Helper()
	runRoot := filepath.Join(campaignRoot, fmt.Sprintf("run-%04d", frame.sequence))
	if err := os.Mkdir(runRoot, 0o700); err != nil {
		t.Fatal("REL001_KILL_RUN_ROOT_CREATE_FAILED")
	}
	t.Cleanup(func() { removeREL001KillRunRoot(t, runRoot) })

	processTemp := filepath.Join(runRoot, "temp")
	if err := os.Mkdir(processTemp, 0o700); err != nil {
		t.Fatal("REL001_KILL_PROCESS_TEMP_CREATE_FAILED")
	}
	childEnvironment, err := rel001KillChildEnvironment(os.Environ(), processTemp)
	if err != nil {
		t.Fatal(err)
	}
	first := startMindWeaverWithEnvironment(t, artifacts, runRoot, childEnvironment)
	firstPID := first.command.Process.Pid
	if firstPID <= 0 {
		t.Fatal("REL001_KILL_FIRST_PROCESS_ID_INVALID")
	}
	t.Cleanup(func() { strictREL001ProcessCleanup(t, first) })
	firstSession := exchangeSession(t, first)
	conversationID := ""
	var providerAttempts atomic.Int64
	if frame.mutation == rel001KillAskNoContext {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal("REL001_KILL_PROVIDER_LISTEN_FAILED")
		}
		provider := httptest.NewUnstartedServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			providerAttempts.Add(1)
			response.WriteHeader(http.StatusInternalServerError)
		}))
		provider.Listener = listener
		provider.Start()
		t.Cleanup(provider.Close)
		var configured struct {
			Config struct {
				Version int64 `json:"version"`
			} `json:"config"`
		}
		firstSession.json(t, http.MethodPut, "/api/v1/ollama", map[string]any{
			"expectedVersion": int64(0), "endpoint": provider.URL,
			"model": "rel001-no-context", "timeoutMilliseconds": int64(1000),
		}, &configured, http.StatusOK)
		if configured.Config.Version != 1 || providerAttempts.Load() != 0 {
			t.Fatal("REL001_KILL_PROVIDER_FIXTURE_INVALID")
		}
		setup := rel001KillTemplate{
			method: http.MethodPost, path: "/api/v1/conversations", key: rel001KillKey("setup", frame.sequence),
			contentType: "application/json", body: `{"title":"REL001 Ask setup ` + fmt.Sprintf("%04d", frame.sequence) + `"}`,
		}
		identity, created, err := executeREL001KillMutation(firstSession, setup, rel001KillConversation)
		if err != nil || !created {
			t.Fatal("REL001_KILL_ASK_SETUP_FAILED")
		}
		conversationID = identity.primary
	}
	template := rel001KillRequest(frame, conversationID)

	var firstEvent atomic.Int32
	wroteHeaders := make(chan struct{})
	var wroteOnce sync.Once
	response := make(chan rel001KillResponse, 1)
	requestContext, cancelRequest := context.WithCancel(t.Context())
	defer cancelRequest()
	go func() {
		request, err := newREL001KillRequest(firstSession, template)
		if err != nil {
			response <- rel001KillResponse{err: err}
			return
		}
		trace := &httptrace.ClientTrace{WroteHeaders: func() { wroteOnce.Do(func() { close(wroteHeaders) }) }}
		request = request.WithContext(httptrace.WithClientTrace(requestContext, trace))
		identity, _, err := executeREL001KillRequest(firstSession, request, frame.mutation)
		if err == nil {
			firstEvent.CompareAndSwap(0, 1)
		}
		response <- rel001KillResponse{identity: identity, err: err}
	}()
	select {
	case <-wroteHeaders:
	case <-time.After(rel001KillRequestTimeout):
		t.Fatal("REL001_KILL_WROTE_HEADERS_TIMEOUT")
	}
	time.Sleep(frame.delay)
	firstEvent.CompareAndSwap(0, 2)
	first.terminate(t)
	cancelRequest()
	var before rel001KillResponse
	select {
	case before = <-response:
	case <-time.After(rel001KillRequestTimeout):
		t.Fatal("REL001_KILL_REQUEST_DRAIN_TIMEOUT")
	}
	acknowledged := firstEvent.Load() == 1
	if acknowledged && before.err != nil {
		t.Fatal("REL001_KILL_ACK_RESPONSE_INVALID")
	}

	restarted := startMindWeaverWithEnvironment(t, artifacts, runRoot, childEnvironment)
	if restarted.command.Process.Pid <= 0 || restarted.command.Process.Pid == firstPID {
		t.Fatal("REL001_KILL_RESTART_PROCESS_ID_INVALID")
	}
	t.Cleanup(func() { strictREL001ProcessCleanup(t, restarted) })
	restartSession := exchangeSession(t, restarted)
	firstReplay, _, err := executeREL001KillMutation(restartSession, template, frame.mutation)
	if err != nil {
		t.Fatalf("REL001_KILL_FIRST_REPLAY_FAILED mutation=%s error=%s", rel001KillMutationNames[frame.mutation], err)
	}
	if acknowledged && firstReplay != before.identity {
		t.Fatalf("REL001_KILL_FIRST_REPLAY_IDENTITY_DRIFT mutation=%s primary=%t secondary=%t tertiary=%t status=%t limitation=%t code=%t",
			rel001KillMutationNames[frame.mutation], firstReplay.primary == before.identity.primary,
			firstReplay.secondary == before.identity.secondary, firstReplay.tertiary == before.identity.tertiary,
			firstReplay.status == before.identity.status, firstReplay.limitation == before.identity.limitation,
			firstReplay.code == before.identity.code)
	}
	secondReplay, created, err := executeREL001KillMutation(restartSession, template, frame.mutation)
	if err != nil || created || secondReplay != firstReplay {
		t.Fatal("REL001_KILL_EXACT_REPLAY_FAILED")
	}
	verifyREL001KillOracle(t, restartSession, frame, template, firstReplay, conversationID, providerAttempts.Load())
	firstSession.client.CloseIdleConnections()
	restartSession.client.CloseIdleConnections()
	restarted.terminate(t)
	removeREL001KillRunRoot(t, runRoot)
	return acknowledged
}

func rel001KillRequest(frame rel001KillFrame, conversationID string) rel001KillTemplate {
	sequence := fmt.Sprintf("%04d", frame.sequence)
	template := rel001KillTemplate{key: rel001KillKey("mutation", frame.sequence)}
	switch frame.mutation {
	case rel001KillCollection:
		template.method, template.path, template.contentType = http.MethodPost, "/api/v1/collections", "application/json"
		template.expected = "REL001 collection " + sequence
		template.body = `{"name":` + strconv.Quote(template.expected) + `}`
	case rel001KillConversation:
		template.method, template.path, template.contentType = http.MethodPost, "/api/v1/conversations", "application/json"
		template.expected = "REL001 conversation " + sequence
		template.body = `{"title":` + strconv.Quote(template.expected) + `}`
	case rel001KillUpload:
		template.method, template.path, template.contentType = http.MethodPost, "/api/v1/documents/upload", "application/octet-stream"
		template.title, template.filename = "REL001 upload "+sequence, "rel001-"+sequence+".txt"
		template.body = "relkill" + sequence + " durable upload source"
	case rel001KillAskNoContext:
		template.method, template.path, template.contentType = http.MethodPost, "/api/v1/ask", "application/json"
		body, _ := json.Marshal(struct {
			ConversationID   string `json:"conversationId"`
			ExpectedRevision int64  `json:"expectedRevision"`
			Question         string `json:"question"`
		}{conversationID, 0, "relkill absent source " + sequence})
		template.body = string(body)
	}
	return template
}

func rel001KillKey(purpose string, sequence int) string {
	return fmt.Sprintf("rel001-%s-%04d", purpose, sequence)
}

func newREL001KillRequest(session *apiSession, template rel001KillTemplate) (*http.Request, error) {
	request, err := http.NewRequest(template.method, session.origin+template.path, strings.NewReader(template.body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Origin", session.origin)
	request.Header.Set("X-MindWeaver-CSRF", session.csrf)
	request.Header.Set("Content-Type", template.contentType)
	request.Header.Set("Idempotency-Key", template.key)
	if template.title != "" {
		request.Header.Set("X-MindWeaver-Title-B64", base64.RawURLEncoding.EncodeToString([]byte(template.title)))
		request.Header.Set("X-MindWeaver-Filename-B64", base64.RawURLEncoding.EncodeToString([]byte(template.filename)))
	}
	return request, nil
}

func executeREL001KillMutation(session *apiSession, template rel001KillTemplate, mutation rel001KillMutation) (rel001KillIdentity, bool, error) {
	request, err := newREL001KillRequest(session, template)
	if err != nil {
		return rel001KillIdentity{}, false, errors.New("REL001_KILL_MUTATION_REQUEST_INVALID")
	}
	return executeREL001KillRequest(session, request, mutation)
}

func executeREL001KillRequest(session *apiSession, request *http.Request, mutation rel001KillMutation) (rel001KillIdentity, bool, error) {
	response, err := session.client.Do(request)
	if err != nil {
		return rel001KillIdentity{}, false, errors.New("REL001_KILL_MUTATION_TRANSPORT_FAILED")
	}
	var identity rel001KillIdentity
	created := false
	switch mutation {
	case rel001KillCollection:
		var wire struct {
			Collection struct {
				ID string `json:"id"`
			} `json:"collection"`
			Created bool `json:"created"`
		}
		err = decodeREL001KillJSON(response.Body, &wire)
		identity.primary, created = wire.Collection.ID, wire.Created
	case rel001KillConversation:
		var wire struct {
			Conversation struct {
				ID string `json:"id"`
			} `json:"conversation"`
			Created bool `json:"created"`
		}
		err = decodeREL001KillJSON(response.Body, &wire)
		identity.primary, created = wire.Conversation.ID, wire.Created
	case rel001KillUpload:
		var wire struct {
			DocumentID string `json:"documentId"`
			RevisionID string `json:"revisionId"`
			JobID      string `json:"jobId"`
			Created    bool   `json:"created"`
		}
		err = decodeREL001KillJSON(response.Body, &wire)
		identity, created = rel001KillIdentity{primary: wire.DocumentID, secondary: wire.RevisionID, tertiary: wire.JobID}, wire.Created
	case rel001KillAskNoContext:
		var wire struct {
			Answer struct {
				ID                   string `json:"id"`
				ConversationID       string `json:"conversationId"`
				Status               string `json:"status"`
				LimitationCode       string `json:"limitationCode"`
				ErrorCode            string `json:"errorCode"`
				ConversationRevision int64  `json:"conversationRevision"`
			} `json:"answer"`
		}
		err = decodeREL001KillJSON(response.Body, &wire)
		identity = rel001KillIdentity{
			primary: wire.Answer.ID, secondary: wire.Answer.ConversationID,
			tertiary: strconv.FormatInt(wire.Answer.ConversationRevision, 10),
			status:   wire.Answer.Status, limitation: wire.Answer.LimitationCode, code: wire.Answer.ErrorCode,
		}
	default:
		_ = response.Body.Close()
		return identity, false, errors.New("REL001_KILL_MUTATION_INVALID")
	}
	closeErr := response.Body.Close()
	if err != nil {
		return rel001KillIdentity{}, false, errors.New("REL001_KILL_MUTATION_RESPONSE_DECODE_INVALID")
	}
	if closeErr != nil {
		return rel001KillIdentity{}, false, errors.New("REL001_KILL_MUTATION_RESPONSE_CLOSE_FAILED")
	}
	if !validREL001KillStatus(response.StatusCode, mutation) {
		return rel001KillIdentity{}, false, fmt.Errorf("REL001_KILL_MUTATION_STATUS_INVALID_%d", response.StatusCode)
	}
	if !validREL001KillIdentity(identity, mutation) {
		return rel001KillIdentity{}, false, fmt.Errorf("REL001_KILL_MUTATION_IDENTITY_INVALID status=%s limitation=%s code=%s revision=%s",
			identity.status, identity.limitation, identity.code, identity.tertiary)
	}
	return identity, created, nil
}

func validREL001KillStatus(status int, mutation rel001KillMutation) bool {
	switch mutation {
	case rel001KillCollection, rel001KillConversation:
		return status == http.StatusOK || status == http.StatusCreated
	case rel001KillUpload:
		return status == http.StatusOK || status == http.StatusAccepted
	case rel001KillAskNoContext:
		return status == http.StatusOK
	}
	return false
}

func validREL001KillIdentity(identity rel001KillIdentity, mutation rel001KillMutation) bool {
	if !validREL001KillIdentifier(identity.primary) {
		return false
	}
	switch mutation {
	case rel001KillCollection, rel001KillConversation:
		return identity.secondary == "" && identity.tertiary == ""
	case rel001KillUpload:
		return validREL001KillIdentifier(identity.secondary) && validREL001KillIdentifier(identity.tertiary)
	case rel001KillAskNoContext:
		return validREL001KillIdentifier(identity.secondary) && identity.tertiary == "1" &&
			(identity.status == "refused" && identity.limitation == "NO_CONTEXT" && identity.code == "" ||
				identity.status == "failed" && identity.limitation == "OUTCOME_UNCERTAIN" && identity.code == "OUTCOME_UNCERTAIN")
	}
	return false
}

func validREL001KillIdentifier(value string) bool {
	return value != "" && len(value) <= 128 && strings.TrimSpace(value) == value
}

func decodeREL001KillJSON(reader io.Reader, output any) error {
	raw, err := io.ReadAll(io.LimitReader(reader, rel001KillMaximumBody+1))
	if err != nil || len(raw) > rel001KillMaximumBody {
		return errors.New("REL001_KILL_RESPONSE_BOUND_EXCEEDED")
	}
	return json.Unmarshal(raw, output)
}

func verifyREL001KillOracle(t *testing.T, session *apiSession, frame rel001KillFrame, template rel001KillTemplate, identity rel001KillIdentity, conversationID string, providerAttempts int64) {
	t.Helper()
	switch frame.mutation {
	case rel001KillCollection:
		var wire struct {
			Collections []struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				Revision int64  `json:"revision"`
			} `json:"collections"`
		}
		rel001KillGetJSON(t, session, "/api/v1/collections?limit=100", &wire)
		if template.expected == "" || len(wire.Collections) != 1 || wire.Collections[0].ID != identity.primary ||
			wire.Collections[0].Name != template.expected || wire.Collections[0].Revision <= 0 {
			t.Fatal("REL001_KILL_COLLECTION_ORACLE_FAILED")
		}
	case rel001KillConversation:
		var wire struct {
			Conversations []struct {
				ID       string `json:"id"`
				Title    string `json:"title"`
				Revision int64  `json:"revision"`
			} `json:"conversations"`
		}
		rel001KillGetJSON(t, session, "/api/v1/conversations?limit=50", &wire)
		if template.expected == "" || len(wire.Conversations) != 1 || wire.Conversations[0].ID != identity.primary ||
			wire.Conversations[0].Title != template.expected || wire.Conversations[0].Revision != 0 {
			t.Fatal("REL001_KILL_CONVERSATION_ORACLE_FAILED")
		}
	case rel001KillUpload:
		waitREL001KillJob(t, session, identity.tertiary)
		var documents struct {
			Documents []struct {
				ID               string `json:"id"`
				Title            string `json:"title"`
				MediaType        string `json:"mediaType"`
				ActiveRevisionID string `json:"activeRevisionId"`
				IngestionStatus  string `json:"ingestionStatus"`
			} `json:"documents"`
		}
		rel001KillGetJSON(t, session, "/api/v1/documents?limit=100", &documents)
		if len(documents.Documents) != 1 || documents.Documents[0].ID != identity.primary ||
			documents.Documents[0].Title != template.title || documents.Documents[0].MediaType != "text/plain" ||
			documents.Documents[0].ActiveRevisionID != identity.secondary || documents.Documents[0].IngestionStatus != "succeeded" {
			t.Fatal("REL001_KILL_UPLOAD_ORACLE_FAILED")
		}
		var search struct {
			Hits []struct {
				DocumentID string `json:"documentId"`
				RevisionID string `json:"revisionId"`
			} `json:"hits"`
		}
		query := strings.Fields(template.body)[0]
		rel001KillGetJSON(t, session, "/api/v1/search?q="+url.QueryEscape(query)+"&limit=20&offset=0", &search)
		if len(search.Hits) != 1 || search.Hits[0].DocumentID != identity.primary || search.Hits[0].RevisionID != identity.secondary {
			t.Fatal("REL001_KILL_UPLOAD_SEARCH_ORACLE_FAILED")
		}
	case rel001KillAskNoContext:
		if providerAttempts != 0 {
			t.Fatal("REL001_KILL_NO_CONTEXT_CALLED_PROVIDER")
		}
		diagnostics := readDiagnostics(t, session)
		wantReconciled := int64(0)
		if identity.status == "failed" {
			wantReconciled = 1
		}
		if diagnostics.ReconciledPendingAnswers != wantReconciled {
			t.Fatal("REL001_KILL_ASK_RECONCILIATION_ORACLE_FAILED")
		}
		var conversations struct {
			Conversations []struct {
				ID       string `json:"id"`
				Title    string `json:"title"`
				Revision int64  `json:"revision"`
			} `json:"conversations"`
		}
		rel001KillGetJSON(t, session, "/api/v1/conversations?limit=50", &conversations)
		if identity.secondary != conversationID || len(conversations.Conversations) != 1 ||
			conversations.Conversations[0].ID != conversationID ||
			conversations.Conversations[0].Title != fmt.Sprintf("REL001 Ask setup %04d", frame.sequence) ||
			conversations.Conversations[0].Revision != 1 {
			t.Fatal("REL001_KILL_ASK_CONVERSATION_ORACLE_FAILED")
		}
		var messages struct {
			Messages []struct {
				ID             string `json:"id"`
				ConversationID string `json:"conversationId"`
				Role           string `json:"role"`
				Status         string `json:"status"`
				Content        string `json:"content"`
				LimitationCode string `json:"limitationCode"`
				ErrorCode      string `json:"errorCode"`
				Ordinal        int    `json:"ordinal"`
			} `json:"messages"`
		}
		rel001KillGetJSON(t, session, "/api/v1/conversations/messages?conversation_id="+url.QueryEscape(conversationID)+"&limit=50", &messages)
		var askBody struct {
			Question string `json:"question"`
		}
		_ = json.Unmarshal([]byte(template.body), &askBody)
		if len(messages.Messages) != 2 || !validREL001KillIdentifier(messages.Messages[0].ID) ||
			messages.Messages[0].ID == identity.primary || messages.Messages[0].ConversationID != conversationID ||
			messages.Messages[0].Ordinal != 1 || messages.Messages[0].Role != "user" || messages.Messages[0].Status != "completed" ||
			messages.Messages[0].Content != askBody.Question || messages.Messages[0].LimitationCode != "" || messages.Messages[0].ErrorCode != "" ||
			messages.Messages[1].ID != identity.primary ||
			messages.Messages[1].ConversationID != conversationID || messages.Messages[1].Ordinal != 2 ||
			messages.Messages[1].Role != "assistant" || messages.Messages[1].Content == "" || len(messages.Messages[1].Content) > 256<<10 ||
			messages.Messages[1].Status != identity.status || messages.Messages[1].LimitationCode != identity.limitation ||
			messages.Messages[1].ErrorCode != identity.code {
			t.Fatal("REL001_KILL_ASK_MESSAGES_ORACLE_FAILED")
		}
	}
}

func rel001KillGetJSON(t *testing.T, session *apiSession, path string, output any) {
	t.Helper()
	response := session.request(t, http.MethodGet, path, nil, "")
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || decodeREL001KillJSON(response.Body, output) != nil {
		t.Fatal("REL001_KILL_GET_ORACLE_FAILED")
	}
}

func waitREL001KillJob(t *testing.T, session *apiSession, jobID string) {
	t.Helper()
	deadline := time.Now().Add(rel001KillRequestTimeout)
	for {
		var wire struct {
			Job struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"job"`
		}
		rel001KillGetJSON(t, session, "/api/v1/jobs?id="+url.QueryEscape(jobID), &wire)
		if wire.Job.ID != jobID {
			t.Fatal("REL001_KILL_JOB_IDENTITY_FAILED")
		}
		if wire.Job.Status == "succeeded" {
			return
		}
		if wire.Job.Status == "failed" || wire.Job.Status == "cancelled" || time.Now().After(deadline) {
			t.Fatal("REL001_KILL_JOB_TERMINAL_INVALID")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func strictREL001ProcessCleanup(t *testing.T, process *runningApp) {
	t.Helper()
	if process == nil || process.command == nil {
		t.Error("REL001_KILL_CLEANUP_PROCESS_INVALID")
		return
	}
	process.mu.Lock()
	defer process.mu.Unlock()
	if process.finished {
		return
	}
	if process.command.Process == nil {
		t.Error("REL001_KILL_CLEANUP_PROCESS_MISSING")
		return
	}
	killErr := process.command.Process.Kill()
	waitErr, reaped := waitForProcess(process.command, processReapTimeout)
	process.finished = true
	var exitErr *exec.ExitError
	if killErr != nil || !reaped || waitErr == nil || !errors.As(waitErr, &exitErr) || exitErr.ExitCode() == 0 {
		t.Error("REL001_KILL_CLEANUP_FAILED")
	}
}

func removeREL001KillRunRoot(t *testing.T, root string) {
	t.Helper()
	if err := os.RemoveAll(root); err != nil {
		t.Error("REL001_KILL_RUN_ROOT_CLEANUP_FAILED")
		return
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Error("REL001_KILL_RUN_ROOT_REMAINS")
	}
}
