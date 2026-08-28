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
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	rel001KillCampaignEnvironment = "MW_REL001_HTTP_KILL_CAMPAIGN"
	rel001KillCampaignOptIn       = "RUN_1024_V2"
	rel001KillCampaignRuns        = 1024
	rel001KillEarlyDelayLimit     = 2 * time.Millisecond
	rel001KillMaximumDelay        = 100 * time.Millisecond
	rel001KillRequestTimeout      = 10 * time.Second
	rel001KillMaximumBody         = 1 << 20
	rel001KillPlanSHA256          = "70f1d8bf4971192da9db3304fbdf62ea5d9f4d258897a1a6c2664973df6c7906"
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

// rel001DecisionLinearization records only the order between a completely
// decoded response and the controller decision to terminate. The subsequent
// Process.Kill and non-zero Wait are separately required for every frame.
type rel001DecisionLinearization int32

const (
	rel001DecisionLinearizationUnset rel001DecisionLinearization = iota
	rel001ResponseBeforeDecision
	rel001DecisionBeforeResponse
)

var rel001DecisionLinearizationNames = [...]string{
	"", "response-before-decision", "decision-before-response",
}

type rel001KillFrame struct {
	sequence int
	mutation rel001KillMutation
	delay    time.Duration
	early    bool
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

type rel001CatalogItem struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Title    string `json:"title"`
	Revision int64  `json:"revision"`
}

type rel001MutationWire struct {
	Collection   rel001CatalogItem `json:"collection"`
	Conversation rel001CatalogItem `json:"conversation"`
	DocumentID   string            `json:"documentId"`
	RevisionID   string            `json:"revisionId"`
	JobID        string            `json:"jobId"`
	Created      bool              `json:"created"`
	Answer       struct {
		ID                   string `json:"id"`
		ConversationID       string `json:"conversationId"`
		Status               string `json:"status"`
		LimitationCode       string `json:"limitationCode"`
		ErrorCode            string `json:"errorCode"`
		ConversationRevision int64  `json:"conversationRevision"`
	} `json:"answer"`
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
	if got := rel001KillPlanDigest(frames); got != rel001KillPlanSHA256 {
		t.Fatalf("REL001_KILL_PLAN_DIGEST_DRIFT got=%s", got)
	}
	selectedFrames := frames
	if !enabled {
		smoke := [4][2]rel001KillFrame{}
		selected := [4][2]bool{}
		for _, frame := range frames {
			mode := 0
			if frame.early {
				mode = 1
			}
			if !selected[frame.mutation][mode] {
				smoke[frame.mutation][mode] = frame
				selected[frame.mutation][mode] = true
			}
		}
		selectedFrames = make([]rel001KillFrame, 0, len(smoke)*2)
		for mutation, modes := range selected {
			if !modes[0] || !modes[1] {
				t.Fatalf("REL001_KILL_SMOKE_PLAN_INCOMPLETE mutation=%s", rel001KillMutationNames[mutation])
			}
			selectedFrames = append(selectedFrames, smoke[mutation][:]...)
		}
	}
	root := moduleRoot(t)
	artifacts := buildREL001KillBinary(t, root)
	campaignRoot := t.TempDir()
	decisionOrders := [4][2]int{}
	for _, frame := range selectedFrames {
		frame := frame
		passed := t.Run(fmt.Sprintf("%04d-%s", frame.sequence, rel001KillMutationNames[frame.mutation]), func(t *testing.T) {
			decisionOrder := runREL001KillFrame(t, artifacts, campaignRoot, frame)
			if frame.early && decisionOrder != rel001DecisionBeforeResponse {
				t.Fatal("REL001_KILL_EARLY_DECISION_LINEARIZATION_FAILED")
			}
			decisionOrders[frame.mutation][int(decisionOrder)-1]++
		})
		if !passed {
			t.Fatalf("REL001_KILL_CAMPAIGN_STOPPED_AT_FRAME sequence=%d mutation=%s",
				frame.sequence, rel001KillMutationNames[frame.mutation])
		}
	}
	if entries, err := os.ReadDir(campaignRoot); err != nil || len(entries) != 0 {
		t.Fatal("REL001_KILL_CAMPAIGN_CLEANUP_INCOMPLETE")
	}
	if enabled {
		total := 0
		for mutation, counts := range decisionOrders {
			total += counts[0] + counts[1]
			if counts[0]+counts[1] != rel001KillCampaignRuns/len(decisionOrders) {
				t.Fatalf("REL001_KILL_MUTATION_COUNT_DRIFT mutation=%s total=%d",
					rel001KillMutationNames[mutation], counts[0]+counts[1])
			}
			if counts[0] == 0 || counts[1] == 0 {
				t.Fatalf("REL001_KILL_DECISION_LINEARIZATION_MISSING mutation=%s response_before_decision=%d decision_before_response=%d",
					rel001KillMutationNames[mutation], counts[0], counts[1])
			}
		}
		if total != rel001KillCampaignRuns {
			t.Fatalf("REL001_KILL_CAMPAIGN_COUNT_DRIFT total=%d", total)
		}
		t.Logf("REL001_KILL_CAMPAIGN_RESULT total=%d collection-create.response-before-decision=%d collection-create.decision-before-response=%d conversation-create.response-before-decision=%d conversation-create.decision-before-response=%d txt-upload.response-before-decision=%d txt-upload.decision-before-response=%d ask-no-context.response-before-decision=%d ask-no-context.decision-before-response=%d early=8,2,2,4 plan_sha256=%s",
			total, decisionOrders[0][0], decisionOrders[0][1], decisionOrders[1][0], decisionOrders[1][1],
			decisionOrders[2][0], decisionOrders[2][1], decisionOrders[3][0], decisionOrders[3][1], rel001KillPlanSHA256)
	}
}

func TestREL001KillPlanAndChildEnvironmentAreFailClosed(t *testing.T) {
	frames := rel001KillFrames(t)
	if got := rel001KillPlanDigest(frames); got != rel001KillPlanSHA256 {
		t.Fatalf("REL001_KILL_PLAN_DIGEST_DRIFT got=%s", got)
	}
	early := [4]int{}
	for _, frame := range frames {
		if frame.early != (frame.delay < rel001KillEarlyDelayLimit) {
			t.Fatal("REL001_KILL_EARLY_MODE_DRIFT")
		}
		if frame.early {
			early[frame.mutation]++
		}
	}
	wantEarly := [4]int{8, 2, 2, 4}
	for mutation, count := range early {
		if count != wantEarly[mutation] || count >= rel001KillCampaignRuns/4 {
			t.Fatalf("REL001_KILL_EARLY_COVERAGE_INVALID mutation=%s got=%d want=%d",
				rel001KillMutationNames[mutation], count, wantEarly[mutation])
		}
	}
	if rel001DecisionLinearizationNames[rel001ResponseBeforeDecision] != "response-before-decision" ||
		rel001DecisionLinearizationNames[rel001DecisionBeforeResponse] != "decision-before-response" {
		t.Fatal("REL001_KILL_DECISION_LINEARIZATION_NAMES_DRIFT")
	}
	for _, first := range []rel001DecisionLinearization{rel001ResponseBeforeDecision, rel001DecisionBeforeResponse} {
		var state atomic.Int32
		if !claimREL001DecisionLinearization(&state, first) {
			t.Fatal("REL001_KILL_DECISION_LINEARIZATION_FIRST_CLAIM_REJECTED")
		}
		second := rel001ResponseBeforeDecision
		if first == second {
			second = rel001DecisionBeforeResponse
		}
		if claimREL001DecisionLinearization(&state, second) || rel001DecisionLinearization(state.Load()) != first {
			t.Fatal("REL001_KILL_DECISION_LINEARIZATION_SECOND_CLAIM_CHANGED_OUTCOME")
		}
	}
	var invalid atomic.Int32
	if claimREL001DecisionLinearization(&invalid, rel001DecisionLinearizationUnset) || invalid.Load() != 0 {
		t.Fatal("REL001_KILL_DECISION_LINEARIZATION_INVALID_CLAIM_ACCEPTED")
	}
	assertSelection := func(environment []string, want, wantError bool) {
		got, err := selectREL001KillCampaign(environment)
		if got != want || (err != nil) != wantError {
			t.Fatalf("selection=%t error=%v", got, err)
		}
	}
	assertSelection(nil, false, false)
	assertSelection([]string{rel001KillCampaignEnvironment + "=" + rel001KillCampaignOptIn}, true, false)
	assertSelection([]string{rel001KillCampaignEnvironment + "=RUN"}, false, true)
	assertSelection([]string{"MW_REL001_HTTP_KILL_EXTRA=1"}, false, true)
	assertSelection([]string{rel001KillCampaignEnvironment + "=" + rel001KillCampaignOptIn,
		strings.ToLower(rel001KillCampaignEnvironment) + "=" + rel001KillCampaignOptIn}, false, true)
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
	wantEnvironment := []string{"SystemRoot=C:\\Windows", "WINDIR=C:\\Windows",
		"TEMP=" + temporaryDirectory, "TMP=" + temporaryDirectory}
	if !slices.Equal(environment, wantEnvironment) {
		t.Fatal("REL001_KILL_CHILD_ENVIRONMENT_NOT_SANITIZED")
	}
	if _, err := rel001KillChildEnvironment([]string{"SystemRoot=C:\\Windows", "systemroot=C:\\Other"}, `C:\temp`); err == nil {
		t.Fatal("REL001_KILL_CHILD_ENVIRONMENT_DUPLICATE_ACCEPTED")
	}
}

func claimREL001DecisionLinearization(state *atomic.Int32, outcome rel001DecisionLinearization) bool {
	if state == nil || outcome != rel001ResponseBeforeDecision && outcome != rel001DecisionBeforeResponse {
		return false
	}
	return state.CompareAndSwap(int32(rel001DecisionLinearizationUnset), int32(outcome))
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
		frames[sequence].early = frames[sequence].delay < rel001KillEarlyDelayLimit
	}
	return frames
}

func rel001KillPlanDigest(frames []rel001KillFrame) string {
	digest := sha256.New()
	_, _ = digest.Write([]byte("mindweaver.rel001.http-kill-plan/v2\x00"))
	var encoded [10]byte
	for _, frame := range frames {
		binary.BigEndian.PutUint32(encoded[:4], uint32(frame.sequence))
		encoded[4] = byte(frame.mutation)
		binary.BigEndian.PutUint32(encoded[5:], uint32(frame.delay/time.Microsecond))
		if frame.early {
			encoded[9] = 1
		} else {
			encoded[9] = 0
		}
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

func runREL001KillFrame(t *testing.T, artifacts builtArtifacts, campaignRoot string, frame rel001KillFrame) rel001DecisionLinearization {
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
	t.Cleanup(func() {
		if err := stopREL001Process(first, true); err != nil {
			t.Error("REL001_KILL_CLEANUP_FAILED")
		}
	})
	firstSession := exchangeSession(t, first)
	t.Cleanup(firstSession.client.CloseIdleConnections)
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
			method: http.MethodPost, path: "/api/v1/conversations", key: fmt.Sprintf("rel001-setup-%04d", frame.sequence),
			contentType: "application/json", body: `{"title":"REL001 Ask setup ` + fmt.Sprintf("%04d", frame.sequence) + `"}`,
		}
		identity, created, err := executeREL001KillMutation(firstSession, setup, rel001KillConversation)
		if err != nil || !created {
			t.Fatal("REL001_KILL_ASK_SETUP_FAILED")
		}
		conversationID = identity.primary
	}
	template := rel001KillRequest(frame, conversationID)
	var decisionOrder atomic.Int32
	requestWriteResult := make(chan error, 1)
	firstResponseByte := make(chan struct{}, 1)
	releaseResponse := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(releaseResponse)
		}
	}()
	response := make(chan rel001KillResponse, 1)
	requestContext, cancelRequest := context.WithCancel(t.Context())
	defer cancelRequest()
	go func() {
		request, err := newREL001KillRequest(firstSession, template)
		if err != nil {
			response <- rel001KillResponse{err: err}
			return
		}
		trace := &httptrace.ClientTrace{WroteRequest: func(info httptrace.WroteRequestInfo) {
			select {
			case requestWriteResult <- info.Err:
			default:
			}
		}, GotFirstResponseByte: func() {
			if frame.early {
				select {
				case firstResponseByte <- struct{}{}:
				default:
				}
				<-releaseResponse
			}
		}}
		request = request.WithContext(httptrace.WithClientTrace(requestContext, trace))
		identity, _, err := executeREL001KillRequest(firstSession, request, frame.mutation)
		if err == nil {
			claimREL001DecisionLinearization(&decisionOrder, rel001ResponseBeforeDecision)
		}
		response <- rel001KillResponse{identity: identity, err: err}
	}()
	select {
	case writeErr := <-requestWriteResult:
		if writeErr != nil {
			t.Fatal("REL001_KILL_REQUEST_WRITE_FAILED")
		}
	case <-time.After(rel001KillRequestTimeout):
		t.Fatal("REL001_KILL_REQUEST_WRITE_TIMEOUT")
	}
	if frame.early {
		select {
		case <-firstResponseByte:
		case <-time.After(rel001KillRequestTimeout):
			t.Fatal("REL001_KILL_FIRST_RESPONSE_BYTE_TIMEOUT")
		}
	} else {
		time.Sleep(frame.delay)
	}
	claimREL001DecisionLinearization(&decisionOrder, rel001DecisionBeforeResponse)
	if err := stopREL001Process(first, true); err != nil {
		t.Fatal("REL001_KILL_REQUIRED_TERMINATION_FAILED")
	}
	if frame.early {
		close(releaseResponse)
		released = true
	}
	cancelRequest()
	var firstResponse rel001KillResponse
	select {
	case firstResponse = <-response:
	case <-time.After(rel001KillRequestTimeout):
		t.Fatal("REL001_KILL_REQUEST_DRAIN_TIMEOUT")
	}
	decisionLinearization := rel001DecisionLinearization(decisionOrder.Load())
	if decisionLinearization != rel001ResponseBeforeDecision && decisionLinearization != rel001DecisionBeforeResponse {
		t.Fatal("REL001_KILL_DECISION_LINEARIZATION_INVALID")
	}
	if decisionLinearization == rel001ResponseBeforeDecision && firstResponse.err != nil {
		t.Fatal("REL001_KILL_RESPONSE_BEFORE_DECISION_INVALID")
	}
	restarted := startMindWeaverWithEnvironment(t, artifacts, runRoot, childEnvironment)
	if restarted.command.Process.Pid <= 0 || restarted.command.Process.Pid == firstPID {
		t.Fatal("REL001_KILL_RESTART_PROCESS_ID_INVALID")
	}
	t.Cleanup(func() {
		if err := stopREL001Process(restarted, true); err != nil {
			t.Error("REL001_KILL_CLEANUP_FAILED")
		}
	})
	restartSession := exchangeSession(t, restarted)
	t.Cleanup(restartSession.client.CloseIdleConnections)
	firstReplay, _, err := executeREL001KillMutation(restartSession, template, frame.mutation)
	if err != nil {
		t.Fatalf("REL001_KILL_FIRST_REPLAY_FAILED mutation=%s error=%s", rel001KillMutationNames[frame.mutation], err)
	}
	if firstResponse.err == nil && firstReplay != firstResponse.identity {
		t.Fatalf("REL001_KILL_FIRST_REPLAY_IDENTITY_DRIFT mutation=%s primary=%t secondary=%t tertiary=%t status=%t limitation=%t code=%t",
			rel001KillMutationNames[frame.mutation], firstReplay.primary == firstResponse.identity.primary,
			firstReplay.secondary == firstResponse.identity.secondary, firstReplay.tertiary == firstResponse.identity.tertiary,
			firstReplay.status == firstResponse.identity.status, firstReplay.limitation == firstResponse.identity.limitation,
			firstReplay.code == firstResponse.identity.code)
	}
	secondReplay, created, err := executeREL001KillMutation(restartSession, template, frame.mutation)
	if err != nil || created || secondReplay != firstReplay {
		t.Fatal("REL001_KILL_EXACT_REPLAY_FAILED")
	}
	verifyREL001KillOracle(t, restartSession, frame, template, firstReplay, conversationID, providerAttempts.Load())
	firstSession.client.CloseIdleConnections()
	restartSession.client.CloseIdleConnections()
	if err := stopREL001Process(restarted, true); err != nil {
		t.Fatal("REL001_KILL_REQUIRED_TERMINATION_FAILED")
	}
	removeREL001KillRunRoot(t, runRoot)
	return decisionLinearization
}

func rel001KillRequest(frame rel001KillFrame, conversationID string) rel001KillTemplate {
	sequence := fmt.Sprintf("%04d", frame.sequence)
	template := rel001KillTemplate{key: fmt.Sprintf("rel001-mutation-%04d", frame.sequence)}
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
	var wire rel001MutationWire
	err = decodeREL001KillJSON(response.Body, &wire)
	var identity rel001KillIdentity
	switch mutation {
	case rel001KillCollection:
		identity.primary = wire.Collection.ID
	case rel001KillConversation:
		identity.primary = wire.Conversation.ID
	case rel001KillUpload:
		identity = rel001KillIdentity{primary: wire.DocumentID, secondary: wire.RevisionID, tertiary: wire.JobID}
	case rel001KillAskNoContext:
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
	validStatus := response.StatusCode == http.StatusOK ||
		response.StatusCode == http.StatusCreated && (mutation == rel001KillCollection || mutation == rel001KillConversation) ||
		response.StatusCode == http.StatusAccepted && mutation == rel001KillUpload
	if !validStatus {
		return rel001KillIdentity{}, false, fmt.Errorf("REL001_KILL_MUTATION_STATUS_INVALID_%d", response.StatusCode)
	}
	if !validREL001KillIdentity(identity, mutation) {
		return rel001KillIdentity{}, false, fmt.Errorf("REL001_KILL_MUTATION_IDENTITY_INVALID status=%s limitation=%s code=%s revision=%s",
			identity.status, identity.limitation, identity.code, identity.tertiary)
	}
	return identity, wire.Created, nil
}

func validREL001KillIdentity(identity rel001KillIdentity, mutation rel001KillMutation) bool {
	valid := func(value string) bool { return value != "" && len(value) <= 128 && strings.TrimSpace(value) == value }
	if !valid(identity.primary) {
		return false
	}
	switch mutation {
	case rel001KillCollection, rel001KillConversation:
		return identity.secondary == "" && identity.tertiary == ""
	case rel001KillUpload:
		return valid(identity.secondary) && valid(identity.tertiary)
	case rel001KillAskNoContext:
		return valid(identity.secondary) && identity.tertiary == "1" &&
			(identity.status == "refused" && identity.limitation == "NO_CONTEXT" && identity.code == "" ||
				identity.status == "failed" && identity.limitation == "OUTCOME_UNCERTAIN" && identity.code == "OUTCOME_UNCERTAIN")
	}
	return false
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
			Collections []rel001CatalogItem `json:"collections"`
		}
		rel001KillGetJSON(t, session, "/api/v1/collections?limit=100", &wire)
		if template.expected == "" || len(wire.Collections) != 1 || wire.Collections[0].ID != identity.primary || wire.Collections[0].Name != template.expected || wire.Collections[0].Revision <= 0 {
			t.Fatal("REL001_KILL_COLLECTION_ORACLE_FAILED")
		}
	case rel001KillConversation:
		var wire struct {
			Conversations []rel001CatalogItem `json:"conversations"`
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
			Conversations []rel001CatalogItem `json:"conversations"`
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
		if len(messages.Messages) != 2 || messages.Messages[0].ID == "" || len(messages.Messages[0].ID) > 128 || strings.TrimSpace(messages.Messages[0].ID) != messages.Messages[0].ID ||
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
