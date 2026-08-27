// Package browserqualification owns the fail-closed browser facets of
// SEC-001/UI-001/UI-002 qualification. It never downloads a browser.
package browserqualification

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"runtime"
	"time"
	"unicode/utf8"
)

const (
	approvalSchemaVersion = 1
	reportSchemaVersion   = 2
	qualificationName     = "UI-001/UI-002"
	maxApprovalBytes      = 16 << 10
	maxReportBytes        = 32 << 10
	minObservedProcesses  = 3
	maxObservedProcesses  = 64
)

//go:embed approval.v1.json
var embeddedApproval []byte

type BlockerCode string

const (
	BlockerArtifactNotApproved       BlockerCode = "BROWSER_ARTIFACT_NOT_APPROVED"
	BlockerArtifactBundleInvalid     BlockerCode = "BROWSER_ARTIFACT_BUNDLE_INVALID"
	BlockerProcessSandboxUnavailable BlockerCode = "BROWSER_PROCESS_SANDBOX_NOT_IMPLEMENTED"
	BlockerLaunchProfileNotApproved  BlockerCode = "BROWSER_LAUNCH_PROFILE_NOT_APPROVED"
	BlockerControlledHarness         BlockerCode = "CONTROLLED_HARNESS_NOT_QUALIFIED"
)

// Approval is opaque: only a repository-owned, strictly parsed document can
// create it. The checked-in V1 document deliberately has no approved entry.
type Approval struct {
	artifact *artifactApproval
}

type approvalWire struct {
	SchemaVersion int                `json:"schemaVersion"`
	Artifacts     []artifactApproval `json:"artifacts"`
}

type artifactApproval struct {
	ID         string         `json:"id"`
	OS         string         `json:"os"`
	Arch       string         `json:"arch"`
	Browser    binaryApproval `json:"browser"`
	Driver     binaryApproval `json:"driver"`
	MindWeaver binaryApproval `json:"mindweaver"`
}

type binaryApproval struct {
	FileName string `json:"fileName"`
	SHA256   string `json:"sha256"`
	Size     int64  `json:"size"`
	Version  string `json:"version"`
}

func ParseEmbeddedApproval() (Approval, error) {
	return ParseApproval(embeddedApproval)
}

// ParseApproval rejects duplicate keys, trailing values, unknown fields,
// oversized input, ambiguous platform entries, and unsafe artifact identity.
func ParseApproval(data []byte) (Approval, error) {
	if len(data) == 0 || len(data) > maxApprovalBytes || !utf8.Valid(data) || rejectDuplicateJSONKeys(data) != nil {
		return Approval{}, errors.New("browser qualification: invalid approval document")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var wire approvalWire
	if err := decoder.Decode(&wire); err != nil || requireJSONEOF(decoder) != nil {
		return Approval{}, errors.New("browser qualification: invalid approval document")
	}
	if wire.SchemaVersion != approvalSchemaVersion || wire.Artifacts == nil {
		return Approval{}, errors.New("browser qualification: unsupported approval document")
	}
	if len(wire.Artifacts) == 0 {
		return Approval{}, nil
	}
	if len(wire.Artifacts) != 1 || validateArtifactApproval(wire.Artifacts[0]) != nil {
		return Approval{}, errors.New("browser qualification: invalid artifact approval")
	}
	artifact := wire.Artifacts[0]
	return Approval{artifact: &artifact}, nil
}

type ScenarioResult struct {
	AcceptanceID     string `json:"acceptanceId"`
	ID               string `json:"id"`
	Status           string `json:"status"`
	Code             string `json:"code"`
	ScreenshotSHA256 string `json:"screenshotSha256,omitempty"`
	ScreenshotBytes  int64  `json:"screenshotBytes,omitempty"`
	TraceSHA256      string `json:"traceSha256,omitempty"`
	TraceBytes       int64  `json:"traceBytes,omitempty"`
}

type scenarioRequirement struct {
	AcceptanceID string
	ID           string
}

var requiredScenarios = []scenarioRequirement{
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

func RequiredScenarios() []string {
	result := make([]string, len(requiredScenarios))
	for index, requirement := range requiredScenarios {
		result[index] = requirement.ID
	}
	return result
}

// processBoundary is the sole package-owned point that may eventually build
// or launch mindweaver.exe, fake Ollama, WebDriver, or a browser. It remains
// unexported so another package cannot inject synthetic PASS evidence.
type processBoundary interface {
	Run(context.Context) (processEvidence, BlockerCode, error)
}

type processEvidence struct {
	ApprovalID              string
	BrowserSHA256           string
	DriverSHA256            string
	BrowserVersion          string
	DriverVersion           string
	ExecutableSHA256        string
	PolicySHA256            string
	RootLineageSHA256       string
	DescendantLineageSHA256 string
	Scenarios               []ScenarioResult
	CleanupStatus           string
	CleanupProcesses        int
	CleanupActiveProcesses  int
	SessionClosed           bool
	ArtifactsReverified     bool
	realProof               *realQualificationProof
}

// Only a future Windows Job/ACL-backed implementation in this package may
// construct this proof. Controlled harnesses intentionally leave it nil.
type realQualificationProof struct{}

type artifactEvidence struct {
	ApprovalID     string `json:"approvalId"`
	BrowserSHA256  string `json:"browserSha256"`
	DriverSHA256   string `json:"driverSha256"`
	BrowserVersion string `json:"browserVersion"`
	DriverVersion  string `json:"driverVersion"`
}

// Report is opaque outside the package. Its wire vocabulary cannot carry
// paths, credentials, cookies, CSRF values, prompts, source text, model output,
// environment variables, or child-process output.
type Report struct {
	wire reportWire
}

type reportWire struct {
	SchemaVersion           int               `json:"schemaVersion"`
	Qualification           string            `json:"qualification"`
	Status                  string            `json:"status"`
	Code                    string            `json:"code"`
	SourceRevision          string            `json:"sourceRevision"`
	Platform                string            `json:"platform"`
	StartedAt               string            `json:"startedAt"`
	CompletedAt             string            `json:"completedAt"`
	Artifacts               *artifactEvidence `json:"artifacts,omitempty"`
	ExecutableSHA256        string            `json:"executableSha256,omitempty"`
	PolicySHA256            string            `json:"policySha256,omitempty"`
	RootLineageSHA256       string            `json:"rootProcessLineageSha256,omitempty"`
	DescendantLineageSHA256 string            `json:"descendantProcessLineageSha256,omitempty"`
	Scenarios               []ScenarioResult  `json:"scenarios"`
	SessionClosed           bool              `json:"webdriverSessionClosed"`
	ArtifactsReverified     bool              `json:"artifactsReverified"`
	CleanupStatus           string            `json:"cleanupStatus"`
	CleanupSHA256           string            `json:"cleanupReceiptSha256,omitempty"`
	CleanupProcesses        int               `json:"cleanupOsTotalProcessCount"`
	CleanupActiveProcesses  int               `json:"cleanupOsActiveProcessCount"`
}

type RunOptions struct {
	SourceRevision string
	BundleRoot     string
	Now            func() time.Time
}

// RunQualification is the public fail-closed entry point. Without repository
// approval and a real package-owned process proof it cannot return PASS.
func RunQualification(ctx context.Context, approval Approval, options RunOptions) Report {
	return runQualification(ctx, approval, options, defaultProcessBoundary(approval, options))
}

func runQualification(ctx context.Context, approval Approval, options RunOptions, processes processBoundary) Report {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	started := now().UTC()
	if approval.artifact == nil {
		return blockedReport(options.SourceRevision, started, now().UTC(), BlockerArtifactNotApproved)
	}
	if processes == nil {
		return blockedReport(options.SourceRevision, started, now().UTC(), BlockerProcessSandboxUnavailable)
	}
	evidence, blocker, err := processes.Run(ctx)
	completed := now().UTC()
	if err != nil {
		return failedReport(options.SourceRevision, started, completed)
	}
	if blocker == BlockerControlledHarness && validHarnessEvidence(evidence) {
		return harnessReport(options.SourceRevision, started, completed, evidence)
	}
	if blocker != "" {
		return blockedReport(options.SourceRevision, started, completed, blocker)
	}
	if !validProcessEvidence(evidence) {
		return failedReport(options.SourceRevision, started, completed)
	}
	wire := reportWire{
		SchemaVersion: reportSchemaVersion, Qualification: qualificationName, Status: "PASS", Code: "QUALIFIED",
		SourceRevision: options.SourceRevision, Platform: runtime.GOOS + "/" + runtime.GOARCH,
		StartedAt: started.Format(time.RFC3339Nano), CompletedAt: completed.Format(time.RFC3339Nano),
		Artifacts: &artifactEvidence{
			ApprovalID: evidence.ApprovalID, BrowserSHA256: evidence.BrowserSHA256, DriverSHA256: evidence.DriverSHA256,
			BrowserVersion: evidence.BrowserVersion, DriverVersion: evidence.DriverVersion,
		},
		ExecutableSHA256: evidence.ExecutableSHA256, PolicySHA256: evidence.PolicySHA256,
		RootLineageSHA256: evidence.RootLineageSHA256, DescendantLineageSHA256: evidence.DescendantLineageSHA256,
		Scenarios: append([]ScenarioResult(nil), evidence.Scenarios...), CleanupStatus: evidence.CleanupStatus,
		CleanupProcesses: evidence.CleanupProcesses, CleanupActiveProcesses: evidence.CleanupActiveProcesses,
		SessionClosed: evidence.SessionClosed, ArtifactsReverified: evidence.ArtifactsReverified,
	}
	wire.CleanupSHA256 = reportReceiptDigest(wire)
	return Report{wire: wire}
}

func blockedReport(revision string, started, completed time.Time, code BlockerCode) Report {
	scenarios := make([]ScenarioResult, len(requiredScenarios))
	for index, requirement := range requiredScenarios {
		scenarios[index] = ScenarioResult{AcceptanceID: requirement.AcceptanceID, ID: requirement.ID, Status: "NOT_RUN", Code: "PREREQUISITE_BLOCKED"}
	}
	return Report{wire: reportWire{
		SchemaVersion: reportSchemaVersion, Qualification: qualificationName, Status: "BLOCKED", Code: string(code),
		SourceRevision: revision, Platform: runtime.GOOS + "/" + runtime.GOARCH,
		StartedAt: started.Format(time.RFC3339Nano), CompletedAt: completed.Format(time.RFC3339Nano),
		Scenarios: scenarios, CleanupStatus: "NOT_STARTED",
	}}
}

func failedReport(revision string, started, completed time.Time) Report {
	scenarios := make([]ScenarioResult, len(requiredScenarios))
	for index, requirement := range requiredScenarios {
		scenarios[index] = ScenarioResult{AcceptanceID: requirement.AcceptanceID, ID: requirement.ID, Status: "FAIL", Code: "QUALIFICATION_ABORTED"}
	}
	return Report{wire: reportWire{
		SchemaVersion: reportSchemaVersion, Qualification: qualificationName, Status: "FAIL", Code: "PROCESS_OR_EVIDENCE_FAILED",
		SourceRevision: revision, Platform: runtime.GOOS + "/" + runtime.GOARCH,
		StartedAt: started.Format(time.RFC3339Nano), CompletedAt: completed.Format(time.RFC3339Nano),
		Scenarios: scenarios, CleanupStatus: "FAILED_OR_UNKNOWN",
	}}
}

func harnessReport(revision string, started, completed time.Time, evidence processEvidence) Report {
	wire := reportWire{
		SchemaVersion: reportSchemaVersion, Qualification: qualificationName, Status: "BLOCKED", Code: string(BlockerControlledHarness),
		SourceRevision: revision, Platform: runtime.GOOS + "/" + runtime.GOARCH,
		StartedAt: started.Format(time.RFC3339Nano), CompletedAt: completed.Format(time.RFC3339Nano),
		Artifacts: &artifactEvidence{
			ApprovalID: evidence.ApprovalID, BrowserSHA256: evidence.BrowserSHA256, DriverSHA256: evidence.DriverSHA256,
			BrowserVersion: evidence.BrowserVersion, DriverVersion: evidence.DriverVersion,
		},
		ExecutableSHA256: evidence.ExecutableSHA256, RootLineageSHA256: evidence.RootLineageSHA256,
		Scenarios: append([]ScenarioResult(nil), evidence.Scenarios...), CleanupStatus: evidence.CleanupStatus,
		CleanupProcesses: evidence.CleanupProcesses, CleanupActiveProcesses: evidence.CleanupActiveProcesses,
		SessionClosed: evidence.SessionClosed, ArtifactsReverified: evidence.ArtifactsReverified,
	}
	wire.CleanupSHA256 = reportReceiptDigest(wire)
	return Report{wire: wire}
}

func validProcessEvidence(evidence processEvidence) bool {
	if !stableToken(evidence.ApprovalID, 64) || !lowerSHA256(evidence.BrowserSHA256) || !lowerSHA256(evidence.DriverSHA256) ||
		!lowerSHA256(evidence.ExecutableSHA256) || !stableVersion(evidence.BrowserVersion) || !stableVersion(evidence.DriverVersion) ||
		!lowerSHA256(evidence.PolicySHA256) || !lowerSHA256(evidence.RootLineageSHA256) ||
		!lowerSHA256(evidence.DescendantLineageSHA256) || evidence.CleanupStatus != "PASS" ||
		!validObservedProcessCount(evidence.CleanupProcesses) || evidence.CleanupActiveProcesses != 0 ||
		!evidence.SessionClosed || !evidence.ArtifactsReverified || len(evidence.Scenarios) != len(requiredScenarios) || evidence.realProof == nil {
		return false
	}
	for index, result := range evidence.Scenarios {
		if !validScenarioResult(result, requiredScenarios[index], "PASS", "QUALIFIED", true) ||
			!lowerSHA256(result.ScreenshotSHA256) || result.ScreenshotBytes <= 0 || result.ScreenshotBytes > maxWebDriverBytes ||
			!lowerSHA256(result.TraceSHA256) || result.TraceBytes <= 0 || result.TraceBytes > maxTraceBytes {
			return false
		}
	}
	return true
}

func validHarnessEvidence(evidence processEvidence) bool {
	if evidence.realProof != nil || !stableToken(evidence.ApprovalID, 64) || !lowerSHA256(evidence.BrowserSHA256) ||
		!lowerSHA256(evidence.DriverSHA256) || !lowerSHA256(evidence.ExecutableSHA256) ||
		!stableVersion(evidence.BrowserVersion) || !stableVersion(evidence.DriverVersion) ||
		evidence.CleanupStatus != "HARNESS_PASS" || !lowerSHA256(evidence.RootLineageSHA256) ||
		!validObservedProcessCount(evidence.CleanupProcesses) || evidence.CleanupActiveProcesses != 0 ||
		!evidence.SessionClosed || !evidence.ArtifactsReverified || len(evidence.Scenarios) != len(requiredScenarios) {
		return false
	}
	for index, result := range evidence.Scenarios {
		if !validScenarioResult(result, requiredScenarios[index], "HARNESS_PASS", "NOT_QUALIFIED", true) ||
			!lowerSHA256(result.ScreenshotSHA256) || result.ScreenshotBytes <= 0 || result.ScreenshotBytes > maxWebDriverBytes ||
			!lowerSHA256(result.TraceSHA256) || result.TraceBytes <= 0 || result.TraceBytes > maxTraceBytes {
			return false
		}
	}
	return true
}

func MarshalReport(report Report) ([]byte, error) {
	if err := report.validate(); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(report.wire, "", "  ")
	if err != nil || len(data)+1 > maxReportBytes {
		return nil, errors.New("browser qualification: report serialization failed")
	}
	return append(data, '\n'), nil
}

func (report Report) validate() error {
	wire := report.wire
	if wire.SchemaVersion != reportSchemaVersion || wire.Qualification != qualificationName || !lowerSHA1(wire.SourceRevision) ||
		wire.Platform != runtime.GOOS+"/"+runtime.GOARCH || len(wire.Scenarios) != len(requiredScenarios) {
		return errors.New("browser qualification: invalid report")
	}
	started, startErr := time.Parse(time.RFC3339Nano, wire.StartedAt)
	completed, completeErr := time.Parse(time.RFC3339Nano, wire.CompletedAt)
	if startErr != nil || completeErr != nil || completed.Before(started) {
		return errors.New("browser qualification: invalid report")
	}
	for index, scenario := range wire.Scenarios {
		if scenario.AcceptanceID != requiredScenarios[index].AcceptanceID || scenario.ID != requiredScenarios[index].ID {
			return errors.New("browser qualification: invalid report")
		}
	}
	switch wire.Status {
	case "BLOCKED":
		if !allowedBlockerCode(wire.Code) {
			return errors.New("browser qualification: invalid blocked report")
		}
		if wire.Code == string(BlockerControlledHarness) {
			if wire.Artifacts == nil || !validArtifactEvidence(*wire.Artifacts) || !lowerSHA256(wire.ExecutableSHA256) ||
				wire.PolicySHA256 != "" || !lowerSHA256(wire.RootLineageSHA256) || wire.DescendantLineageSHA256 != "" ||
				wire.CleanupStatus != "HARNESS_PASS" || !lowerSHA256(wire.CleanupSHA256) ||
				wire.CleanupSHA256 != reportReceiptDigest(wire) || !validObservedProcessCount(wire.CleanupProcesses) ||
				wire.CleanupActiveProcesses != 0 || !wire.SessionClosed || !wire.ArtifactsReverified {
				return errors.New("browser qualification: invalid blocked report")
			}
			for index, scenario := range wire.Scenarios {
				if !validScenarioResult(scenario, requiredScenarios[index], "HARNESS_PASS", "NOT_QUALIFIED", true) {
					return errors.New("browser qualification: invalid blocked report")
				}
			}
		} else {
			if wire.Artifacts != nil || wire.ExecutableSHA256 != "" || wire.PolicySHA256 != "" ||
				wire.RootLineageSHA256 != "" || wire.DescendantLineageSHA256 != "" || wire.CleanupStatus != "NOT_STARTED" ||
				wire.CleanupSHA256 != "" || wire.CleanupProcesses != 0 || wire.CleanupActiveProcesses != 0 ||
				wire.SessionClosed || wire.ArtifactsReverified {
				return errors.New("browser qualification: invalid blocked report")
			}
			for index, scenario := range wire.Scenarios {
				if !validScenarioResult(scenario, requiredScenarios[index], "NOT_RUN", "PREREQUISITE_BLOCKED", false) {
					return errors.New("browser qualification: invalid blocked report")
				}
			}
		}
	case "PASS":
		if wire.Code != "QUALIFIED" || wire.Artifacts == nil || !validArtifactEvidence(*wire.Artifacts) ||
			!lowerSHA256(wire.ExecutableSHA256) || !lowerSHA256(wire.PolicySHA256) ||
			!lowerSHA256(wire.RootLineageSHA256) || !lowerSHA256(wire.DescendantLineageSHA256) ||
			wire.CleanupStatus != "PASS" || !lowerSHA256(wire.CleanupSHA256) || wire.CleanupSHA256 != reportReceiptDigest(wire) ||
			!validObservedProcessCount(wire.CleanupProcesses) || wire.CleanupActiveProcesses != 0 ||
			!wire.SessionClosed || !wire.ArtifactsReverified {
			return errors.New("browser qualification: invalid pass report")
		}
		for index, scenario := range wire.Scenarios {
			if !validScenarioResult(scenario, requiredScenarios[index], "PASS", "QUALIFIED", true) {
				return errors.New("browser qualification: invalid pass report")
			}
		}
	case "FAIL":
		if wire.Code != "PROCESS_OR_EVIDENCE_FAILED" || wire.Artifacts != nil || wire.ExecutableSHA256 != "" ||
			wire.PolicySHA256 != "" || wire.RootLineageSHA256 != "" || wire.DescendantLineageSHA256 != "" ||
			wire.CleanupStatus != "FAILED_OR_UNKNOWN" || wire.CleanupSHA256 != "" || wire.CleanupProcesses != 0 ||
			wire.CleanupActiveProcesses != 0 || wire.SessionClosed || wire.ArtifactsReverified {
			return errors.New("browser qualification: invalid failure report")
		}
		for index, scenario := range wire.Scenarios {
			if !validScenarioResult(scenario, requiredScenarios[index], "FAIL", "QUALIFICATION_ABORTED", false) {
				return errors.New("browser qualification: invalid failure report")
			}
		}
	default:
		return errors.New("browser qualification: invalid report status")
	}
	return nil
}

func validScenarioResult(result ScenarioResult, requirement scenarioRequirement, status, code string, requireEvidence bool) bool {
	if result.AcceptanceID != requirement.AcceptanceID || result.ID != requirement.ID || result.Status != status || result.Code != code {
		return false
	}
	if !requireEvidence {
		return result.ScreenshotSHA256 == "" && result.ScreenshotBytes == 0 && result.TraceSHA256 == "" && result.TraceBytes == 0
	}
	return lowerSHA256(result.ScreenshotSHA256) && result.ScreenshotBytes > 0 && result.ScreenshotBytes <= maxWebDriverBytes &&
		lowerSHA256(result.TraceSHA256) && result.TraceBytes > 0 && result.TraceBytes <= maxTraceBytes
}

func validObservedProcessCount(value int) bool {
	return value >= minObservedProcesses && value <= maxObservedProcesses
}

func reportReceiptDigest(wire reportWire) string {
	wire.CleanupSHA256 = ""
	data, err := json.Marshal(wire)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func (report Report) Status() string        { return report.wire.Status }
func (report Report) Code() string          { return report.wire.Code }
func (report Report) CleanupStatus() string { return report.wire.CleanupStatus }
func (report Report) Scenarios() []ScenarioResult {
	return append([]ScenarioResult(nil), report.wire.Scenarios...)
}

func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := scanJSONValue(decoder); err != nil {
		return err
	}
	return requireJSONEOF(decoder)
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("invalid JSON object key")
			}
			if _, exists := seen[key]; exists {
				return errors.New("duplicate JSON object key")
			}
			seen[key] = struct{}{}
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	default:
		return errors.New("unexpected JSON delimiter")
	}
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON value")
	}
	return nil
}

func lowerSHA256(value string) bool { return lowerHex(value, 64) }
func lowerSHA1(value string) bool   { return lowerHex(value, 40) }

func lowerHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	_, err := hex.DecodeString(value)
	if err != nil {
		return false
	}
	for _, character := range value {
		if character >= 'A' && character <= 'F' {
			return false
		}
	}
	return true
}

func stableToken(value string, limit int) bool {
	if value == "" || len(value) > limit {
		return false
	}
	for index, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') ||
			(index > 0 && (character == '.' || character == '_' || character == '-')) {
			continue
		}
		return false
	}
	return true
}

func stableVersion(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '.' || character == '_' || character == '-' || character == '+' {
			continue
		}
		return false
	}
	return true
}

func allowedBlockerCode(value string) bool {
	return value == string(BlockerArtifactNotApproved) || value == string(BlockerArtifactBundleInvalid) ||
		value == string(BlockerProcessSandboxUnavailable) || value == string(BlockerLaunchProfileNotApproved) ||
		value == string(BlockerControlledHarness)
}

func validArtifactEvidence(evidence artifactEvidence) bool {
	return stableToken(evidence.ApprovalID, 64) && lowerSHA256(evidence.BrowserSHA256) && lowerSHA256(evidence.DriverSHA256) &&
		stableVersion(evidence.BrowserVersion) && stableVersion(evidence.DriverVersion)
}
