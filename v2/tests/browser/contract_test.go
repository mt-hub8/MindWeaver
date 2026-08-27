package browserqualification

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const testRevision = "0123456789abcdef0123456789abcdef01234567"

const verifierFixtureReceipt = "990739d1510ac5529062fb416490d42aa55951f54037be096c1abbed43540950"

func TestEmbeddedApprovalFailsClosedBeforeArtifactOrProcessBoundary(t *testing.T) {
	approval, err := ParseEmbeddedApproval()
	if err != nil {
		t.Fatal(err)
	}
	processes := &recordingBoundary{}
	missingBundle := filepath.Join(t.TempDir(), "must-not-be-opened")
	report := runQualification(context.Background(), approval, RunOptions{
		SourceRevision: testRevision, BundleRoot: missingBundle, Now: fixedClock(),
	}, processes)
	if processes.calls != 0 {
		t.Fatalf("blocked qualification crossed process boundary %d times", processes.calls)
	}
	if _, err := os.Stat(missingBundle); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty approval touched artifact bundle: %v", err)
	}
	assertBlocked(t, report, BlockerArtifactNotApproved)
}

func TestApprovalParserAcceptsOneExactBundleAndRejectsHostileInput(t *testing.T) {
	approval := testArtifactApproval()
	valid := marshalApproval(t, approval)
	parsed, err := ParseApproval(valid)
	if err != nil || parsed.artifact == nil || parsed.artifact.ID != approval.ID {
		t.Fatalf("valid approval parse = %#v, %v", parsed, err)
	}
	for name, document := range map[string][]byte{
		"empty":                  nil,
		"duplicate key":          []byte(`{"schemaVersion":1,"schemaVersion":1,"artifacts":[]}`),
		"semantic duplicate key": []byte(`{"schemaVersion":1,"SchemaVersion":1,"artifacts":[]}`),
		"case variant root":      bytes.Replace(valid, []byte(`"schemaVersion"`), []byte(`"SchemaVersion"`), 1),
		"case variant artifact":  bytes.Replace(valid, []byte(`"id"`), []byte(`"ID"`), 1),
		"case variant binary":    bytes.Replace(valid, []byte(`"fileName"`), []byte(`"FileName"`), 1),
		"unknown field":          []byte(`{"schemaVersion":1,"artifacts":[],"extra":true}`),
		"trailing value":         append(append([]byte(nil), valid...), []byte(`{}`)...),
		"wrong schema":           []byte(`{"schemaVersion":2,"artifacts":[]}`),
		"missing list":           []byte(`{"schemaVersion":1}`),
		"two artifacts":          marshalApprovalList(t, approval, approval),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseApproval(document); err == nil {
				t.Fatal("hostile approval unexpectedly parsed")
			}
		})
	}
	mutations := map[string]func(*artifactApproval){
		"path":           func(value *artifactApproval) { value.Browser.FileName = `..\browser.exe` },
		"uppercase hash": func(value *artifactApproval) { value.Driver.SHA256 = strings.Repeat("A", 64) },
		"case collision": func(value *artifactApproval) { value.Driver.FileName = strings.ToUpper(value.Browser.FileName) },
		"oversize":       func(value *artifactApproval) { value.MindWeaver.Size = maxApprovedArtifactBytes + 1 },
		"system edge":    func(value *artifactApproval) { value.Browser.FileName = `C:\Program Files\Edge\msedge.exe` },
		"reserved leaf":  func(value *artifactApproval) { value.Browser.FileName = "con.exe" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			candidate := approval
			mutate(&candidate)
			if _, err := ParseApproval(marshalApproval(t, candidate)); err == nil {
				t.Fatal("unsafe artifact approval unexpectedly parsed")
			}
		})
	}
	oversized := append(marshalApproval(t, approval), make([]byte, maxApprovalBytes)...)
	if _, err := ParseApproval(oversized); err == nil {
		t.Fatal("oversized approval unexpectedly parsed")
	}
}

func TestApprovedArtifactDefaultBoundaryReverifiesThenReportsNextPlatformPrerequisite(t *testing.T) {
	approval := testArtifactApproval()
	bundle := writeArtifactBundle(t, approval)
	report := RunQualification(context.Background(), Approval{artifact: &approval}, RunOptions{
		SourceRevision: testRevision, BundleRoot: bundle, Now: fixedClock(),
	})
	expected := BlockerProcessSandboxUnavailable
	if processSandboxAvailable() {
		expected = BlockerLaunchProfileNotApproved
	}
	assertBlocked(t, report, expected)

	if err := os.WriteFile(filepath.Join(bundle, "extra.exe"), []byte("extra"), 0o600); err != nil {
		t.Fatal(err)
	}
	report = RunQualification(context.Background(), Approval{artifact: &approval}, RunOptions{
		SourceRevision: testRevision, BundleRoot: bundle, Now: fixedClock(),
	})
	assertBlocked(t, report, BlockerArtifactBundleInvalid)
}

func TestArtifactBundleRejectsWrongBytesAndHardLinks(t *testing.T) {
	approval := testArtifactApproval()
	bundle := writeArtifactBundle(t, approval)
	if err := os.WriteFile(filepath.Join(bundle, approval.Driver.FileName), []byte(strings.Repeat("z", int(approval.Driver.Size))), 0o600); err != nil {
		t.Fatal(err)
	}
	report := RunQualification(context.Background(), Approval{artifact: &approval}, RunOptions{
		SourceRevision: testRevision, BundleRoot: bundle, Now: fixedClock(),
	})
	assertBlocked(t, report, BlockerArtifactBundleInvalid)

	bundle = writeArtifactBundle(t, approval)
	external := filepath.Join(t.TempDir(), "external-browser.exe")
	if err := os.Rename(filepath.Join(bundle, approval.Browser.FileName), external); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(external, filepath.Join(bundle, approval.Browser.FileName)); err != nil {
		t.Fatal(err)
	}
	report = RunQualification(context.Background(), Approval{artifact: &approval}, RunOptions{
		SourceRevision: testRevision, BundleRoot: bundle, Now: fixedClock(),
	})
	assertBlocked(t, report, BlockerArtifactBundleInvalid)
}

func TestRetainedArtifactCapabilitiesRejectACLExpansionWriteAndReplacement(t *testing.T) {
	approval := testArtifactApproval()
	bundle := writeArtifactBundle(t, approval)
	artifacts, err := openApprovedArtifacts(approval, bundle)
	if err != nil {
		t.Fatal(err)
	}
	browserPath := filepath.Join(bundle, approval.Browser.FileName)
	if err := os.WriteFile(browserPath, []byte(strings.Repeat("x", int(approval.Browser.Size))), 0o600); err == nil {
		_ = artifacts.Close()
		t.Fatal("retained browser artifact allowed a concurrent write")
	}
	if err := os.Rename(browserPath, filepath.Join(bundle, "replacement.exe")); err == nil {
		_ = artifacts.Close()
		t.Fatal("retained browser artifact allowed a concurrent rename")
	}
	if err := artifacts.Reverify(); err != nil {
		_ = artifacts.Close()
		t.Fatalf("retained artifact changed after denied mutation: %v", err)
	}
	if err := expandTestArtifactACL(browserPath); err != nil {
		_ = artifacts.Close()
		t.Fatalf("expand retained artifact ACL: %v", err)
	}
	if err := artifacts.Reverify(); err == nil {
		_ = artifacts.Close()
		t.Fatal("retained artifact accepted an in-flight ACL expansion")
	}
	if err := artifacts.Close(); err != nil {
		t.Fatal(err)
	}

	bundle = writeArtifactBundle(t, approval)
	if err := expandTestArtifactACL(filepath.Join(bundle, approval.Driver.FileName)); err != nil {
		t.Fatal(err)
	}
	report := RunQualification(context.Background(), Approval{artifact: &approval}, RunOptions{
		SourceRevision: testRevision, BundleRoot: bundle, Now: fixedClock(),
	})
	assertBlocked(t, report, BlockerArtifactBundleInvalid)
}

func TestLiteralLoopbackEndpointRejectsAliasesAndDecorations(t *testing.T) {
	for _, endpoint := range []string{
		"http://localhost:4444", "http://[::1]:4444", "https://127.0.0.1:4444",
		"http://127.0.0.1:4444/path", "http://127.0.0.1:4444?x=1", "http://user@127.0.0.1:4444",
		"http://127.0.0.1:04444", "http://127.0.0.1", "http://127.0.0.2:4444",
	} {
		if _, err := literalLoopbackEndpoint(endpoint); err == nil {
			t.Fatalf("unsafe endpoint accepted: %q", endpoint)
		}
	}
	if _, err := literalLoopbackEndpoint("http://127.0.0.1:4444"); err != nil {
		t.Fatal(err)
	}
}

func TestWebDriverTransportOnlyAcceptsBoundedJSONAndRejectsUnsafeResponses(t *testing.T) {
	t.Run("create and delete", func(t *testing.T) {
		requests := make(chan string, 2)
		endpoint := newWebDriverTransportTestEndpoint(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			requests <- request.Method + " " + request.URL.Path
			writer.Header().Set("Content-Type", "application/json")
			switch {
			case request.Method == http.MethodPost && request.URL.Path == "/session":
				_, _ = writer.Write([]byte(`{"value":{"sessionId":"transport-session-1"}}`))
			case request.Method == http.MethodDelete && request.URL.Path == "/session/transport-session-1":
				_, _ = writer.Write([]byte(`{"value":null}`))
			default:
				http.NotFound(writer, request)
			}
		}))
		client := newWebDriverClient(endpoint)
		sessionID, err := client.createSession(context.Background())
		if err != nil || sessionID != "transport-session-1" {
			t.Fatalf("create session = %q, %v", sessionID, err)
		}
		if err := client.deleteSession(context.Background(), sessionID); err != nil {
			t.Fatalf("delete session: %v", err)
		}
		if got := <-requests; got != "POST /session" {
			t.Fatalf("create request = %q", got)
		}
		if got := <-requests; got != "DELETE /session/transport-session-1" {
			t.Fatalf("delete request = %q", got)
		}
	})

	t.Run("redirect", func(t *testing.T) {
		requests := make(chan string, 2)
		endpoint := newWebDriverTransportTestEndpoint(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			requests <- request.URL.Path
			http.Redirect(writer, request, "/redirected", http.StatusTemporaryRedirect)
		}))
		if _, err := newWebDriverClient(endpoint).createSession(context.Background()); err == nil {
			t.Fatal("webdriver redirect was accepted")
		}
		if got := <-requests; got != "/session" {
			t.Fatalf("redirect source request = %q", got)
		}
		select {
		case got := <-requests:
			t.Fatalf("webdriver followed redirect to %q", got)
		default:
		}
	})

	t.Run("non json", func(t *testing.T) {
		endpoint := newWebDriverTransportTestEndpoint(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "text/plain")
			_, _ = writer.Write([]byte(`{"value":{"sessionId":"transport-session-1"}}`))
		}))
		if _, err := newWebDriverClient(endpoint).createSession(context.Background()); err == nil {
			t.Fatal("non-JSON webdriver response was accepted")
		}
	})

	t.Run("oversize", func(t *testing.T) {
		endpoint := newWebDriverTransportTestEndpoint(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write(bytes.Repeat([]byte("x"), maxWebDriverBytes+1))
		}))
		if _, err := newWebDriverClient(endpoint).createSession(context.Background()); err == nil {
			t.Fatal("oversized webdriver response was accepted")
		}
	})
}

func newWebDriverTransportTestEndpoint(t *testing.T, handler http.Handler) *url.URL {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: handler}
	done := make(chan error, 1)
	go func() {
		done <- server.Serve(listener)
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			t.Errorf("shutdown webdriver transport test server: %v", err)
		}
		if err := <-done; err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("webdriver transport test server: %v", err)
		}
	})
	endpoint, err := literalLoopbackEndpoint("http://" + listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return endpoint
}

func TestProcessErrorReportIsContentFree(t *testing.T) {
	forbiddenValues := []string{"vault-path-canary-7d21", "cookie-value-canary-4a91", "csrf-value-canary-55b0", "source-text-canary-916c"}
	approval := testArtifactApproval()
	processes := &recordingBoundary{err: errors.New(strings.Join(forbiddenValues, " "))}
	report := runQualification(context.Background(), Approval{artifact: &approval}, RunOptions{
		SourceRevision: testRevision, Now: fixedClock(),
	}, processes)
	data, err := MarshalReport(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range forbiddenValues {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("report leaked process detail %q", forbidden)
		}
	}
	if len(data) > maxReportBytes {
		t.Fatal("report exceeded its byte bound")
	}
}

func TestRequiredScenarioListCannotBeMutatedByCaller(t *testing.T) {
	first := RequiredScenarios()
	first[0] = "FORGED"
	second := RequiredScenarios()
	if second[0] == "FORGED" || len(second) != len(requiredScenarios) {
		t.Fatal("required scenario contract was mutable")
	}
}

func TestRequiredScenarioGroupsCoverSecurityAndEnabledUIWithoutCLIRecovery(t *testing.T) {
	want := []scenarioRequirement{
		{AcceptanceID: "SEC-001", ID: "SEC001_CSP_ENFORCEMENT"},
		{AcceptanceID: "UI-001", ID: "UI001_ASK_MALFORMED_CITATION"},
		{AcceptanceID: "UI-001", ID: "UI001_ASK_NO_HIT"},
		{AcceptanceID: "UI-001", ID: "UI001_ASK_STRUCTURAL_CITATIONS"},
		{AcceptanceID: "UI-001", ID: "UI001_BOOTSTRAP_ONE_USE"},
		{AcceptanceID: "UI-001", ID: "UI001_CSRF_ROTATION"},
		{AcceptanceID: "UI-001", ID: "UI001_NO_EXTERNAL_NETWORK"},
		{AcceptanceID: "UI-001", ID: "UI001_NO_MODEL_UPLOAD_SEARCH"},
		{AcceptanceID: "UI-001", ID: "UI001_OLLAMA_LOOPBACK_CONFIG"},
		{AcceptanceID: "UI-001", ID: "UI001_OUTCOME_UNCERTAIN_RESTART"},
		{AcceptanceID: "UI-001", ID: "UI001_TWO_TAB_CONCURRENCY"},
		{AcceptanceID: "UI-001", ID: "UI001_BACKUP_CREATE_STATUS_CANCEL"},
		{AcceptanceID: "UI-001", ID: "UI001_BACKUP_LOST_RESPONSE_REPLAY"},
		{AcceptanceID: "UI-001", ID: "UI001_DIAGNOSTICS"},
		{AcceptanceID: "UI-001", ID: "UI001_INGESTION_PROGRESS_RESTART"},
		{AcceptanceID: "UI-002", ID: "UI002_KEYBOARD_FOCUS"},
		{AcceptanceID: "UI-002", ID: "UI002_REFLOW_CONTRAST_MOTION"},
		{AcceptanceID: "UI-002", ID: "UI002_ZH_IME"},
	}
	if len(requiredScenarios) != len(want) {
		t.Fatalf("required scenario count = %d, want %d", len(requiredScenarios), len(want))
	}
	for index := range want {
		if requiredScenarios[index] != want[index] {
			t.Fatalf("required scenario %d = %#v, want %#v", index, requiredScenarios[index], want[index])
		}
		if strings.Contains(requiredScenarios[index].ID, "RECOVERY_VERIFY") || strings.Contains(requiredScenarios[index].ID, "RECOVERY_RESTORE") {
			t.Fatalf("startup-only recovery was presented as a browser scenario: %q", requiredScenarios[index].ID)
		}
	}
}

func TestPassReceiptRejectsPostQualificationMutation(t *testing.T) {
	for name, mutate := range map[string]func(*reportWire){
		"source":         func(wire *reportWire) { wire.SourceRevision = strings.Repeat("1", 40) },
		"artifact":       func(wire *reportWire) { wire.Artifacts.Browser.SHA256 = strings.Repeat("b", 64) },
		"policy":         func(wire *reportWire) { wire.PolicySHA256 = strings.Repeat("b", 64) },
		"root":           func(wire *reportWire) { wire.RootLineageSHA256 = strings.Repeat("b", 64) },
		"descendant":     func(wire *reportWire) { wire.DescendantLineageSHA256 = strings.Repeat("b", 64) },
		"processes":      func(wire *reportWire) { wire.CleanupProcesses++ },
		"session":        func(wire *reportWire) { wire.SessionClosed = false },
		"reverification": func(wire *reportWire) { wire.ArtifactsReverified = false },
		"scenario":       func(wire *reportWire) { wire.Scenarios[0].TraceSHA256 = strings.Repeat("b", 64) },
	} {
		t.Run(name, func(t *testing.T) {
			report := qualifiedReport(t, minObservedProcesses)
			mutate(&report.wire)
			if _, err := MarshalReport(report); err == nil {
				t.Fatalf("post-qualification %s mutation retained a valid receipt", name)
			}
		})
	}
}

func TestReportParserRejectsAmbiguousOrForgedV2JSON(t *testing.T) {
	valid, err := MarshalReport(qualifiedReport(t, minObservedProcesses))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseReport(valid)
	if err != nil || parsed.Status() != "PASS" {
		t.Fatalf("strict parse = %#v, %v", parsed, err)
	}
	replacements := map[string][]byte{
		"duplicate root key":          bytes.Replace(valid, []byte(`  "status": "PASS",`), []byte("  \"status\": \"BLOCKED\",\n  \"status\": \"PASS\","), 1),
		"semantic duplicate root key": bytes.Replace(valid, []byte(`  "status": "PASS",`), []byte("  \"Status\": \"PASS\",\n  \"status\": \"PASS\","), 1),
		"case variant root":           bytes.Replace(valid, []byte(`"status"`), []byte(`"Status"`), 1),
		"case variant artifact":       bytes.Replace(valid, []byte(`"approvalId"`), []byte(`"ApprovalId"`), 1),
		"case variant binary":         bytes.Replace(valid, []byte(`"role"`), []byte(`"Role"`), 1),
		"case variant scenario":       bytes.Replace(valid, []byte(`"acceptanceId"`), []byte(`"AcceptanceID"`), 1),
		"unknown root key":            bytes.Replace(valid, []byte(`  "code": "QUALIFIED",`), []byte("  \"code\": \"QUALIFIED\",\n  \"extra\": true,"), 1),
		"unknown artifact":            bytes.Replace(valid, []byte(`    "approvalId": "controlled-offline-bundle-1",`), []byte("    \"approvalId\": \"controlled-offline-bundle-1\",\n    \"extra\": true,"), 1),
		"unknown scenario":            bytes.Replace(valid, []byte(`      "code": "QUALIFIED",`), []byte("      \"code\": \"QUALIFIED\",\n      \"extra\": true,"), 1),
		"string process count": bytes.Replace(valid, []byte(`  "cleanupOsTotalProcessCount": 3,`),
			[]byte(`  "cleanupOsTotalProcessCount": "3",`), 1),
		"forged receipt": bytes.Replace(valid, []byte(`"cleanupReceiptSha256": "`+qualifiedReport(t, minObservedProcesses).wire.CleanupSHA256+`"`),
			[]byte(`"cleanupReceiptSha256": "`+strings.Repeat("a", 64)+`"`), 1),
		"trailing value": append(append([]byte(nil), valid...), []byte(`{}`)...),
	}
	for name, candidate := range replacements {
		t.Run(name, func(t *testing.T) {
			if bytes.Equal(candidate, valid) {
				t.Fatal("test mutation did not change the report")
			}
			if _, err := parseReport(candidate); err == nil {
				t.Fatal("ambiguous or forged report unexpectedly parsed")
			}
		})
	}
}

func TestCanonicalReceiptMatchesPowerShellVerifierFixture(t *testing.T) {
	hash := strings.Repeat("a", 64)
	artifacts := artifactEvidence{
		ApprovalID: "self-test-approved-bundle",
		Browser:    binaryArtifactEvidence{Role: "browser", FileName: "browser.exe", SHA256: hash, Size: 101, Version: "1.0+browser"},
		Driver:     binaryArtifactEvidence{Role: "driver", FileName: "driver.exe", SHA256: hash, Size: 102, Version: "1.0+driver"},
		MindWeaver: binaryArtifactEvidence{Role: "mindweaver", FileName: "mindweaver.exe", SHA256: hash, Size: 103, Version: "1.0+mindweaver"},
	}
	scenarios := make([]ScenarioResult, len(requiredScenarios))
	for index, requirement := range requiredScenarios {
		scenarios[index] = ScenarioResult{
			AcceptanceID: requirement.AcceptanceID, ID: requirement.ID, Status: "PASS", Code: "QUALIFIED",
			ScreenshotSHA256: hash, ScreenshotBytes: 1, TraceSHA256: hash, TraceBytes: 1,
		}
	}
	wire := reportWire{
		SchemaVersion: reportSchemaVersion, Qualification: qualificationName, Status: "PASS", Code: "QUALIFIED",
		SourceRevision: testRevision, Platform: "windows/amd64",
		StartedAt: "2026-08-27T00:00:00.123456789Z", CompletedAt: "2026-08-27T00:00:01.123456789Z",
		Artifacts: &artifacts, ExecutableSHA256: hash, PolicySHA256: hash, RootLineageSHA256: hash, DescendantLineageSHA256: hash,
		Scenarios: scenarios, SessionClosed: true, ArtifactsReverified: true, CleanupStatus: "PASS",
		CleanupProcesses: 4, CleanupActiveProcesses: 0,
	}
	if got := reportReceiptDigest(wire); got != verifierFixtureReceipt {
		t.Fatalf("canonical verifier fixture receipt = %q", got)
	}
}

func TestSyntheticEvidenceCannotEnterPublicQualification(t *testing.T) {
	approval := testArtifactApproval()
	report := RunQualification(context.Background(), Approval{artifact: &approval}, RunOptions{
		SourceRevision: testRevision, Now: fixedClock(),
	})
	if report.Status() == "PASS" {
		t.Fatalf("public qualification accepted a synthetic verifier fixture: %#v", report)
	}
}

func TestVerifierFixtureCannotEnterQualificationProducer(t *testing.T) {
	approval := testArtifactApproval()
	fixture := qualifiedReport(t, minObservedProcesses).wire
	evidence := processEvidence{
		Artifacts:               *fixture.Artifacts,
		PolicySHA256:            fixture.PolicySHA256,
		RootLineageSHA256:       fixture.RootLineageSHA256,
		DescendantLineageSHA256: fixture.DescendantLineageSHA256,
		Scenarios:               append([]ScenarioResult(nil), fixture.Scenarios...),
		CleanupStatus:           fixture.CleanupStatus,
		CleanupProcesses:        fixture.CleanupProcesses,
		CleanupActiveProcesses:  fixture.CleanupActiveProcesses,
		SessionClosed:           fixture.SessionClosed,
		ArtifactsReverified:     fixture.ArtifactsReverified,
	}
	report := runQualification(context.Background(), Approval{artifact: &approval}, RunOptions{
		SourceRevision: testRevision, Now: fixedClock(),
	}, &recordingBoundary{evidence: evidence})
	if report.Status() != "FAIL" || report.Code() != "PROCESS_OR_EVIDENCE_FAILED" {
		t.Fatalf("verifier-only fixture entered qualification producer: %#v", report)
	}
}

func assertBlocked(t *testing.T, report Report, code BlockerCode) {
	t.Helper()
	if report.Status() != "BLOCKED" || report.Code() != string(code) || report.CleanupStatus() != "NOT_STARTED" {
		t.Fatalf("blocked report = %#v", report)
	}
	for _, scenario := range report.Scenarios() {
		if scenario.Status != "NOT_RUN" || scenario.Code != "PREREQUISITE_BLOCKED" {
			t.Fatalf("blocked scenario = %#v", scenario)
		}
	}
	if _, err := MarshalReport(report); err != nil {
		t.Fatal(err)
	}
}

type recordingBoundary struct {
	calls    int
	evidence processEvidence
	blocker  BlockerCode
	err      error
}

func (boundary *recordingBoundary) Run(context.Context) (processEvidence, BlockerCode, error) {
	boundary.calls++
	return boundary.evidence, boundary.blocker, boundary.err
}

func qualifiedReport(t *testing.T, processCount int) Report {
	t.Helper()
	approval := testArtifactApproval()
	scenarios := make([]ScenarioResult, len(requiredScenarios))
	for index, requirement := range requiredScenarios {
		scenarios[index] = ScenarioResult{
			AcceptanceID: requirement.AcceptanceID, ID: requirement.ID, Status: "PASS", Code: "QUALIFIED",
			ScreenshotSHA256: digest([]byte(requirement.ID + "-screenshot")), ScreenshotBytes: 128,
			TraceSHA256: digest([]byte(requirement.ID + "-trace")), TraceBytes: 96,
		}
	}
	artifacts := artifactEvidenceFromApproval(approval)
	wire := reportWire{
		SchemaVersion: reportSchemaVersion, Qualification: qualificationName, Status: "PASS", Code: "QUALIFIED",
		SourceRevision: testRevision, Platform: runtime.GOOS + "/" + runtime.GOARCH,
		StartedAt: time.Unix(1, 0).UTC().Format(time.RFC3339Nano), CompletedAt: time.Unix(2, 0).UTC().Format(time.RFC3339Nano),
		Artifacts: &artifacts, ExecutableSHA256: approval.MindWeaver.SHA256,
		PolicySHA256:      digest([]byte("approved-browser-launch-profile-and-network-policy-v2")),
		RootLineageSHA256: digest([]byte("observed-root-process-lineage-v2")), DescendantLineageSHA256: digest([]byte("observed-descendant-process-lineage-v2")),
		Scenarios: scenarios, CleanupStatus: "PASS", CleanupProcesses: processCount, CleanupActiveProcesses: 0,
		SessionClosed: true, ArtifactsReverified: true,
	}
	wire.CleanupSHA256 = reportReceiptDigest(wire)
	return Report{wire: wire}
}

func testArtifactApproval() artifactApproval {
	browser := []byte("controlled-browser-artifact-v1")
	driver := []byte("controlled-driver-artifact-v1")
	mindweaver := []byte("controlled-mindweaver-artifact-v1")
	return artifactApproval{
		ID: "controlled-offline-bundle-1", OS: "windows", Arch: "amd64",
		Browser:    binaryApproval{FileName: "browser.exe", SHA256: digest(browser), Size: int64(len(browser)), Version: "1.0.0"},
		Driver:     binaryApproval{FileName: "driver.exe", SHA256: digest(driver), Size: int64(len(driver)), Version: "1.0.0"},
		MindWeaver: binaryApproval{FileName: "mindweaver.exe", SHA256: digest(mindweaver), Size: int64(len(mindweaver)), Version: "1.0.0"},
	}
}

func writeArtifactBundle(t *testing.T, approval artifactApproval) string {
	t.Helper()
	root := t.TempDir()
	contents := map[string][]byte{
		approval.Browser.FileName:    []byte("controlled-browser-artifact-v1"),
		approval.Driver.FileName:     []byte("controlled-driver-artifact-v1"),
		approval.MindWeaver.FileName: []byte("controlled-mindweaver-artifact-v1"),
	}
	for name, content := range contents {
		if err := os.WriteFile(filepath.Join(root, name), content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	protectTestArtifactBundle(t, root, approval)
	verifyTestArtifactBundleProtection(t, root, approval)
	return root
}

func marshalApproval(t *testing.T, approval artifactApproval) []byte {
	t.Helper()
	return marshalApprovalList(t, approval)
}

func marshalApprovalList(t *testing.T, approvals ...artifactApproval) []byte {
	t.Helper()
	data, err := json.Marshal(approvalWire{SchemaVersion: approvalSchemaVersion, Artifacts: approvals})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func digest(data []byte) string {
	value := sha256.Sum256(data)
	return fmt.Sprintf("%x", value[:])
}

func fixedClock() func() time.Time {
	times := []time.Time{time.Unix(1, 0).UTC(), time.Unix(2, 0).UTC()}
	index := 0
	return func() time.Time {
		value := times[index%len(times)]
		index++
		return value
	}
}
