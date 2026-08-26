package browserqualification

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

const testRevision = "0123456789abcdef0123456789abcdef01234567"

func TestEmbeddedApprovalFailsClosed(t *testing.T) {
	approval, err := ParseEmbeddedApproval()
	if err != nil {
		t.Fatal(err)
	}
	processes := &recordingBoundary{}
	report := runQualification(context.Background(), approval, RunOptions{SourceRevision: testRevision, Now: fixedClock()}, processes)
	if processes.calls != 0 {
		t.Fatalf("blocked qualification crossed process boundary %d times", processes.calls)
	}
	assertBlocked(t, report, BlockerArtifactNotApproved)
}

func TestApprovalParserRejectsHostileOrUnimplementedInput(t *testing.T) {
	valid := `{"schemaVersion":1,"artifacts":[]}`
	for name, document := range map[string]string{
		"empty":          "",
		"duplicate key":  `{"schemaVersion":1,"schemaVersion":1,"artifacts":[]}`,
		"unknown field":  `{"schemaVersion":1,"artifacts":[],"extra":true}`,
		"trailing value": valid + `{}`,
		"wrong schema":   `{"schemaVersion":2,"artifacts":[]}`,
		"missing list":   `{"schemaVersion":1}`,
		"artifact entry": `{"schemaVersion":1,"artifacts":[{}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseApproval([]byte(document)); err == nil {
				t.Fatal("hostile approval unexpectedly parsed")
			}
		})
	}
	oversized := append([]byte(valid), make([]byte, maxApprovalBytes)...)
	if _, err := ParseApproval(oversized); err == nil {
		t.Fatal("oversized approval unexpectedly parsed")
	}
}

func TestApprovedArtifactStillCannotStartWithoutRunner(t *testing.T) {
	processes := &recordingBoundary{}
	report := runQualification(context.Background(), Approval{browserApproved: true}, RunOptions{
		SourceRevision: testRevision, Now: fixedClock(),
	}, nil)
	if processes.calls != 0 {
		t.Fatal("nil runner unexpectedly crossed process boundary")
	}
	assertBlocked(t, report, BlockerRunnerNotImplemented)
}

func TestPassRequiresCompleteProcessEvidenceAndCleanup(t *testing.T) {
	processes := &recordingBoundary{evidence: passingEvidence()}
	report := runQualification(context.Background(), Approval{browserApproved: true}, RunOptions{
		SourceRevision: testRevision, Now: fixedClock(),
	}, processes)
	if processes.calls != 1 || report.Status() != "PASS" {
		t.Fatalf("process calls/status = %d/%s", processes.calls, report.Status())
	}
	if _, err := MarshalReport(report); err != nil {
		t.Fatal(err)
	}

	processes.evidence.Scenarios = processes.evidence.Scenarios[:len(processes.evidence.Scenarios)-1]
	report = runQualification(context.Background(), Approval{browserApproved: true}, RunOptions{
		SourceRevision: testRevision, Now: fixedClock(),
	}, processes)
	if report.Status() != "FAIL" || report.Code() != "PROCESS_OR_EVIDENCE_FAILED" || report.CleanupStatus() != "FAILED_OR_UNKNOWN" {
		t.Fatalf("incomplete evidence report = %#v", report)
	}
}

func TestProcessErrorAndBlockedReportAreContentFree(t *testing.T) {
	forbiddenValues := []string{"vault-path-canary-7d21", "cookie-value-canary-4a91", "csrf-value-canary-55b0", "source-text-canary-916c"}
	secret := strings.Join(forbiddenValues, " ")
	processes := &recordingBoundary{err: errors.New(secret)}
	report := runQualification(context.Background(), Approval{browserApproved: true}, RunOptions{
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

	approval, err := ParseEmbeddedApproval()
	if err != nil {
		t.Fatal(err)
	}
	blocked := RunQualification(context.Background(), approval, RunOptions{SourceRevision: testRevision, Now: fixedClock()})
	data, err = MarshalReport(blocked)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range forbiddenValues {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("blocked report contains forbidden field/value %q", forbidden)
		}
	}
}

func TestRequiredScenarioListCannotBeMutatedByCaller(t *testing.T) {
	first := RequiredScenarios()
	first[0] = "FORGED"
	second := RequiredScenarios()
	if second[0] == "FORGED" || len(second) != 13 {
		t.Fatal("required scenario contract was mutable")
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
	err      error
}

func (boundary *recordingBoundary) Run(context.Context) (processEvidence, error) {
	boundary.calls++
	return boundary.evidence, boundary.err
}

func passingEvidence() processEvidence {
	scenarios := make([]ScenarioResult, len(requiredScenarioIDs))
	for index, id := range requiredScenarioIDs {
		scenarios[index] = ScenarioResult{ID: id, Status: "PASS", Code: "QUALIFIED"}
	}
	return processEvidence{
		ApprovalID: "edge-offline-1", BrowserSHA256: strings.Repeat("a", 64), DriverSHA256: strings.Repeat("b", 64),
		BrowserVersion: "1.0.0", DriverVersion: "1.0.0", ExecutableSHA256: strings.Repeat("c", 64),
		Scenarios: scenarios, CleanupStatus: "PASS",
	}
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
