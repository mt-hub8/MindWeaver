package supplychain

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf16"
)

const (
	maxArtifactBytes           int64 = 128 << 20
	maxBuildOutputBytes              = 1 << 20
	maxCLIOutputBytes                = 64 << 10
	prepackageBuildTimeout           = 3 * time.Minute
	prepackageCLITimeout             = 10 * time.Second
	prepackageProcessWaitDelay       = 2 * time.Second
)

type processCompletion string

const (
	processCompleted     processCompletion = "COMPLETED"
	processStartFailed   processCompletion = "START_FAILED"
	processTimedOut      processCompletion = "TIMED_OUT"
	processControlFailed processCompletion = "CONTROL_FAILED"
)

type boundedProcessOutput struct {
	mu       sync.Mutex
	limit    uint64
	total    uint64
	exceeded bool
	data     []byte
	digest   hash.Hash
	cancel   context.CancelFunc
}

type processOutputSummary struct {
	bytes    uint64
	sha256   string
	exceeded bool
	data     []byte
}

type processResult struct {
	completion processCompletion
	exitCode   int
	stdout     processOutputSummary
	stderr     processOutputSummary
}

type sensitiveNeedle struct {
	label     string
	encodings [][]byte
}

type cliContract struct {
	name         string
	artifactName string
	args         []string
	exitCode     int
	stdout       []byte
	stderr       []byte
}

func TestPrepackageDualPEOutputBoundary(t *testing.T) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		t.Fatal("PREPACKAGE_OUTPUT_HOST_UNSUPPORTED")
	}
	root := moduleRoot(t)
	goTool := selectedGoTool(t)
	environment, _ := moduleBuildEnvironment(t, goTool)
	downloadAndVerifyModules(t, goTool, root, environment)
	artifactRoot := filepath.Join(t.TempDir(), "artifacts")
	if err := os.Mkdir(artifactRoot, 0o700); err != nil {
		t.Fatal("ARTIFACT_OUTPUT_DIRECTORY_CREATE_FAILED")
	}

	canaries := map[string]string{
		"SECRET_CANARY": randomCanary(t, "secret"),
		"PROMPT_CANARY": randomCanary(t, "prompt"),
		"SOURCE_CANARY": randomCanary(t, "source"),
		"PATH_CANARY":   randomCanary(t, "path"),
	}
	buildOverrides := make(map[string]string, len(canaries))
	for label, value := range canaries {
		buildOverrides["MW_PREPACKAGE_"+label] = value
	}
	environment = environmentWith(environment, buildOverrides)

	built := make([]builtArtifact, 0, len(artifactContracts))
	for _, contract := range artifactContracts {
		artifact, err := buildArtifact(t, goTool, root, artifactRoot, environment, contract)
		if err != nil {
			t.Fatal(err)
		}
		built = append(built, artifact)
	}
	if err := validateArtifactDirectory(artifactRoot, artifactContracts); err != nil {
		t.Fatal(err)
	}

	needles := hostAndCanaryNeedles(root, artifactRoot, environment, canaries)
	for _, artifact := range built {
		if err := validateArtifactContent(artifact, needles); err != nil {
			t.Fatal(err)
		}
	}

	byName := make(map[string]builtArtifact, len(built))
	for _, artifact := range built {
		byName[artifact.name] = artifact
	}
	missingConfig := filepath.Join(artifactRoot, canaries["PATH_CANARY"], canaries["SOURCE_CANARY"]+".json")
	missingPDF := filepath.Join(artifactRoot, canaries["PATH_CANARY"]+"-"+canaries["SOURCE_CANARY"]+".pdf")
	contracts := []cliContract{
		{
			name: "main.version", artifactName: "mindweaver.exe", args: []string{"version"}, exitCode: 0,
			stdout: []byte("mindweaver 0.1.0-dev (commit=unknown, built=unknown)\n"),
		},
		{
			name: "main.help", artifactName: "mindweaver.exe", args: []string{"help", canaries["PROMPT_CANARY"]}, exitCode: 0,
			stdout: []byte(expectedMindweaverHelp),
		},
		{
			name: "main.unknown", artifactName: "mindweaver.exe", args: []string{canaries["SECRET_CANARY"]}, exitCode: 2,
			stderr: []byte("unknown command; run mindweaver help\n"),
		},
		{
			name: "main.config_missing", artifactName: "mindweaver.exe", args: []string{"config", "check", "-file", missingConfig}, exitCode: 4,
			stderr: []byte("configuration file was not found\n"),
		},
		{
			name: "helper.probe", artifactName: "mindweaver-pdf.exe",
			args: []string{"-probe"}, exitCode: 0, stdout: []byte(expectedPDFProbe),
		},
		{
			name: "helper.missing_input", artifactName: "mindweaver-pdf.exe",
			args: []string{"-input", missingPDF}, exitCode: 20,
		},
		{
			name: "helper.invalid_usage", artifactName: "mindweaver-pdf.exe",
			args: []string{"-input", canaries["SOURCE_CANARY"], canaries["PATH_CANARY"]}, exitCode: 1,
		},
	}
	for _, contract := range contracts {
		artifact, ok := byName[contract.artifactName]
		if !ok {
			t.Fatal("CLI_ARTIFACT_MISSING")
		}
		result := runBoundedProcess(
			t.Context(), prepackageCLITimeout, maxCLIOutputBytes,
			artifact.path, contract.args, artifactRoot, environment,
		)
		if err := validateCLIProcessResult(contract, result, needles); err != nil {
			t.Fatal(err)
		}
	}
	if err := validateArtifactDirectory(artifactRoot, artifactContracts); err != nil {
		t.Fatal(err)
	}
	for _, artifact := range built {
		if err := validateArtifactContent(artifact, needles); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPrepackageOutputBoundaryMutationsFailClosed(t *testing.T) {
	const secret = "MW_PRIVATE_CANARY_0123456789abcdef"

	t.Run("bounded writer cancels at limit", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		output := newBoundedProcessOutput(8, cancel)
		value := []byte("0123456789abcdef")
		written, err := output.Write(value)
		if written != len(value) || !errors.Is(err, errProcessOutputLimit) {
			t.Fatal("bounded output did not stop at its byte limit")
		}
		select {
		case <-ctx.Done():
		default:
			t.Fatal("bounded output did not cancel the process context")
		}
		summary := output.summary()
		if !summary.exceeded || summary.bytes != 9 || len(summary.data) != 8 {
			t.Fatal("bounded output retained bytes beyond its limit")
		}
	})

	t.Run("build failure does not echo diagnostics", func(t *testing.T) {
		result := processResult{
			completion: processCompleted,
			exitCode:   1,
			stdout:     summaryForBytes([]byte(secret), maxBuildOutputBytes),
			stderr:     summaryForBytes([]byte("failure "+secret), maxBuildOutputBytes),
		}
		err := validateBuildProcessResult("mindweaver.exe", result)
		requireSafeBoundaryError(t, err, secret, "BUILD_EXIT_NONZERO")
	})

	t.Run("artifact locator", func(t *testing.T) {
		needle := newSensitiveNeedle("WORKSPACE_ROOT", `C:\private\workspace`)
		artifact := writeSyntheticArtifact(t, "mindweaver.exe", append([]byte("MZ"), needle.encodings[0]...))
		err := validateArtifactContent(artifact, []sensitiveNeedle{needle})
		requireSafeBoundaryError(t, err, `C:\private\workspace`, "WORKSPACE_ROOT")
	})

	t.Run("artifact UTF-16 locator", func(t *testing.T) {
		const locator = `C:\private\utf16-workspace`
		const mixedCaseLocator = `c:\PrIvAtE\UtF16-WoRkSpAcE`
		needle := newSensitiveNeedle("WORKSPACE_ROOT", locator)
		artifact := writeSyntheticArtifact(t, "mindweaver.exe", append([]byte("MZ"), utf16LE(mixedCaseLocator)...))
		err := validateArtifactContent(artifact, []sensitiveNeedle{needle})
		requireSafeBoundaryError(t, err, locator, "WORKSPACE_ROOT")
	})

	t.Run("artifact canary", func(t *testing.T) {
		needle := newSensitiveNeedle("SECRET_CANARY", secret)
		artifact := writeSyntheticArtifact(t, "mindweaver.exe", append([]byte("MZ"), []byte(secret)...))
		err := validateArtifactContent(artifact, []sensitiveNeedle{needle})
		requireSafeBoundaryError(t, err, secret, "SECRET_CANARY")
	})

	t.Run("oversized artifact", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "mindweaver.exe")
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		truncateErr := file.Truncate(maxArtifactBytes + 1)
		closeErr := file.Close()
		if truncateErr != nil || closeErr != nil {
			t.Fatal("create oversized mutation")
		}
		err = validateArtifactContent(builtArtifact{name: "mindweaver.exe", path: path}, nil)
		requireSafeBoundaryError(t, err, path, "ARTIFACT_FILE_INVALID")
	})

	t.Run("non-regular artifact", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "mindweaver.exe")
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		err := validateArtifactContent(builtArtifact{name: "mindweaver.exe", path: path}, nil)
		requireSafeBoundaryError(t, err, path, "ARTIFACT_FILE_INVALID")
	})

	t.Run("extra artifact", func(t *testing.T) {
		root := t.TempDir()
		for _, name := range []string{"mindweaver.exe", "mindweaver-pdf.exe", secret + ".exe"} {
			if err := os.WriteFile(filepath.Join(root, name), []byte("MZ"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		err := validateArtifactDirectory(root, artifactContracts)
		requireSafeBoundaryError(t, err, secret, "ARTIFACT_EXACT_SET_MISMATCH")
	})

	t.Run("oversized CLI output", func(t *testing.T) {
		contract := cliContract{name: "main.version", exitCode: 0}
		result := processResult{
			completion: processCompleted,
			exitCode:   0,
			stdout:     summaryForBytes(bytes.Repeat([]byte("x"), maxCLIOutputBytes+1), maxCLIOutputBytes),
			stderr:     summaryForBytes(nil, maxCLIOutputBytes),
		}
		err := validateCLIProcessResult(contract, result, nil)
		requireSafeBoundaryError(t, err, secret, "CLI_OUTPUT_LIMIT_EXCEEDED")
	})

	t.Run("wrong CLI exit and output stay private", func(t *testing.T) {
		contract := cliContract{name: "main.unknown", exitCode: 2}
		result := processResult{
			completion: processCompleted,
			exitCode:   9,
			stdout:     summaryForBytes([]byte(secret), maxCLIOutputBytes),
			stderr:     summaryForBytes([]byte(secret), maxCLIOutputBytes),
		}
		err := validateCLIProcessResult(contract, result, []sensitiveNeedle{newSensitiveNeedle("SECRET_CANARY", secret)})
		requireSafeBoundaryError(t, err, secret, "CLI_EXIT_MISMATCH")
	})

	t.Run("unexpected CLI output stays private", func(t *testing.T) {
		contract := cliContract{name: "main.version", exitCode: 0, stdout: []byte("expected\n")}
		result := processResult{
			completion: processCompleted,
			exitCode:   0,
			stdout:     summaryForBytes([]byte(secret), maxCLIOutputBytes),
			stderr:     summaryForBytes(nil, maxCLIOutputBytes),
		}
		err := validateCLIProcessResult(contract, result, []sensitiveNeedle{newSensitiveNeedle("SECRET_CANARY", secret)})
		requireSafeBoundaryError(t, err, secret, "CLI_OUTPUT_CONTRACT_MISMATCH")
	})
}

func runBoundedProcess(parent context.Context, timeout time.Duration, outputLimit int, executable string, args []string, directory string, environment []string) processResult {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	stdout := newBoundedProcessOutput(outputLimit, cancel)
	stderr := newBoundedProcessOutput(outputLimit, cancel)
	command := exec.CommandContext(ctx, executable, args...)
	command.Dir = directory
	command.Env = environment
	command.Stdout = stdout
	command.Stderr = stderr
	command.WaitDelay = prepackageProcessWaitDelay
	err := command.Run()
	result := processResult{completion: processCompleted, exitCode: 0, stdout: stdout.summary(), stderr: stderr.summary()}
	if ctx.Err() != nil {
		result.completion = processTimedOut
		result.exitCode = -1
		return result
	}
	if err == nil {
		return result
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.exitCode = exitError.ExitCode()
		return result
	}
	if command.Process == nil {
		result.completion = processStartFailed
	} else {
		result.completion = processControlFailed
	}
	result.exitCode = -1
	return result
}

func newBoundedProcessOutput(limit int, cancel context.CancelFunc) *boundedProcessOutput {
	if limit < 0 {
		limit = 0
	}
	return &boundedProcessOutput{limit: uint64(limit), digest: sha256.New(), cancel: cancel}
}

func (output *boundedProcessOutput) Write(value []byte) (int, error) {
	output.mu.Lock()
	written := len(value)
	if output.exceeded {
		output.mu.Unlock()
		return written, errProcessOutputLimit
	}
	remaining := output.limit - uint64(len(output.data))
	captured := uint64(written)
	if captured > remaining {
		captured = remaining
	}
	if captured > 0 {
		output.data = append(output.data, value[:int(captured)]...)
		_, _ = output.digest.Write(value[:int(captured)])
	}
	if uint64(written) > remaining {
		output.exceeded = true
		output.total = output.limit + 1
	} else {
		output.total += uint64(written)
	}
	exceeded := output.exceeded
	cancel := output.cancel
	output.mu.Unlock()
	if exceeded {
		if cancel != nil {
			cancel()
		}
		return written, errProcessOutputLimit
	}
	return written, nil
}

func (output *boundedProcessOutput) summary() processOutputSummary {
	output.mu.Lock()
	defer output.mu.Unlock()
	return processOutputSummary{
		bytes: output.total, sha256: hex.EncodeToString(output.digest.Sum(nil)), exceeded: output.exceeded,
		data: append([]byte(nil), output.data...),
	}
}

func summaryForBytes(value []byte, limit int) processOutputSummary {
	output := newBoundedProcessOutput(limit, nil)
	_, _ = output.Write(value)
	return output.summary()
}

func validateBuildProcessResult(name string, result processResult) error {
	summary := safeProcessSummary(result)
	if result.stdout.exceeded || result.stderr.exceeded {
		return fmt.Errorf("%s: BUILD_OUTPUT_LIMIT_EXCEEDED %s", name, summary)
	}
	if result.completion != processCompleted {
		return fmt.Errorf("%s: BUILD_%s %s", name, result.completion, summary)
	}
	if result.exitCode != 0 {
		return fmt.Errorf("%s: BUILD_EXIT_NONZERO %s", name, summary)
	}
	if result.stdout.bytes != 0 || result.stderr.bytes != 0 {
		return fmt.Errorf("%s: BUILD_OUTPUT_NOT_EMPTY %s", name, summary)
	}
	return nil
}

func validateCLIProcessResult(contract cliContract, result processResult, needles []sensitiveNeedle) error {
	summary := safeProcessSummary(result)
	if result.stdout.exceeded || result.stderr.exceeded {
		return fmt.Errorf("%s: CLI_OUTPUT_LIMIT_EXCEEDED %s", contract.name, summary)
	}
	if result.completion != processCompleted {
		return fmt.Errorf("%s: CLI_%s %s", contract.name, result.completion, summary)
	}
	if result.exitCode != contract.exitCode {
		return fmt.Errorf("%s: CLI_EXIT_MISMATCH %s", contract.name, summary)
	}
	if !bytes.Equal(result.stdout.data, contract.stdout) || !bytes.Equal(result.stderr.data, contract.stderr) {
		return fmt.Errorf("%s: CLI_OUTPUT_CONTRACT_MISMATCH %s", contract.name, summary)
	}
	for _, stream := range []processOutputSummary{result.stdout, result.stderr} {
		if label := firstSensitiveMatch(stream.data, needles); label != "" {
			return fmt.Errorf("%s: CLI_SENSITIVE_OUTPUT:%s %s", contract.name, label, summary)
		}
	}
	return nil
}

func safeProcessSummary(result processResult) string {
	return fmt.Sprintf(
		"completion=%s exit=%d %s %s",
		result.completion, result.exitCode,
		safeOutputSummary("stdout", result.stdout),
		safeOutputSummary("stderr", result.stderr),
	)
}

func safeOutputSummary(name string, summary processOutputSummary) string {
	if summary.exceeded {
		return fmt.Sprintf("%s_bytes_at_least=%d %s_prefix_sha256=%s", name, summary.bytes, name, summary.sha256)
	}
	return fmt.Sprintf("%s_bytes=%d %s_sha256=%s", name, summary.bytes, name, summary.sha256)
}

func readBoundedArtifact(path string) ([]byte, error) {
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 || before.Size() < 2 || before.Size() > maxArtifactBytes {
		return nil, errors.New("artifact is not a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("artifact open failed")
	}
	after, statErr := file.Stat()
	if statErr != nil || !os.SameFile(before, after) || !after.Mode().IsRegular() || after.Size() != before.Size() {
		_ = file.Close()
		return nil, errors.New("artifact identity changed")
	}
	contents, readErr := io.ReadAll(io.LimitReader(file, maxArtifactBytes+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || int64(len(contents)) != before.Size() || int64(len(contents)) > maxArtifactBytes {
		return nil, errors.New("artifact bounded read failed")
	}
	return contents, nil
}

func validateArtifactContent(artifact builtArtifact, needles []sensitiveNeedle) error {
	contents, err := readBoundedArtifact(artifact.path)
	if err != nil {
		return fmt.Errorf("%s: ARTIFACT_FILE_INVALID", artifact.name)
	}
	digest := sha256.Sum256(contents)
	if artifact.size != 0 && int64(len(contents)) != artifact.size {
		return fmt.Errorf("%s: ARTIFACT_SIZE_CHANGED", artifact.name)
	}
	if artifact.sha256 != "" && hex.EncodeToString(digest[:]) != artifact.sha256 {
		return fmt.Errorf("%s: ARTIFACT_HASH_CHANGED", artifact.name)
	}
	if label := firstSensitiveMatch(contents, needles); label != "" {
		return fmt.Errorf("%s: ARTIFACT_SENSITIVE_BYTES:%s", artifact.name, label)
	}
	return nil
}

func validateArtifactDirectory(root string, contracts []artifactContract) error {
	directory, err := os.Open(root)
	if err != nil {
		return errors.New("ARTIFACT_DIRECTORY_READ_FAILED")
	}
	limit := len(contracts) + 1
	entries := make([]os.DirEntry, 0, limit)
	complete := false
	for len(entries) < limit {
		batch, readErr := directory.ReadDir(limit - len(entries))
		entries = append(entries, batch...)
		if errors.Is(readErr, io.EOF) {
			complete = true
			break
		}
		if readErr != nil || len(batch) == 0 {
			_ = directory.Close()
			return errors.New("ARTIFACT_DIRECTORY_READ_FAILED")
		}
	}
	closeErr := directory.Close()
	if closeErr != nil {
		return errors.New("ARTIFACT_DIRECTORY_READ_FAILED")
	}
	if !complete || len(entries) != len(contracts) {
		return errors.New("ARTIFACT_EXACT_SET_MISMATCH")
	}
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		got = append(got, entry.Name())
	}
	want := make([]string, 0, len(contracts))
	for _, contract := range contracts {
		want = append(want, contract.name)
	}
	sort.Strings(got)
	sort.Strings(want)
	if !slices.Equal(got, want) {
		return errors.New("ARTIFACT_EXACT_SET_MISMATCH")
	}
	return nil
}

func hostAndCanaryNeedles(root, artifactRoot string, environment []string, canaries map[string]string) []sensitiveNeedle {
	values := []struct {
		label string
		value string
	}{
		{"MODULE_ROOT", root},
		{"WORKSPACE_ROOT", filepath.Dir(root)},
		{"ARTIFACT_OUTPUT_ROOT", artifactRoot},
	}
	for _, key := range []string{"GOROOT", "GOMODCACHE", "GOCACHE", "GOTMPDIR", "USERPROFILE", "HOME", "LOCALAPPDATA", "APPDATA", "TEMP", "TMP"} {
		values = append(values, struct {
			label string
			value string
		}{label: key, value: environmentValue(environment, key)})
	}
	canaryLabels := make([]string, 0, len(canaries))
	for label := range canaries {
		canaryLabels = append(canaryLabels, label)
	}
	sort.Strings(canaryLabels)
	for _, label := range canaryLabels {
		values = append(values, struct {
			label string
			value string
		}{label: label, value: canaries[label]})
	}

	result := make([]sensitiveNeedle, 0, len(values))
	seen := make(map[string]bool)
	for _, item := range values {
		value := strings.TrimSpace(item.value)
		if len(value) < 4 {
			continue
		}
		key := strings.ToLower(filepath.Clean(value))
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, newSensitiveNeedle(item.label, value))
	}
	return result
}

func newSensitiveNeedle(label, value string) sensitiveNeedle {
	variants := []string{
		value,
		filepath.Clean(value),
		filepath.ToSlash(value),
		strings.ReplaceAll(value, "/", `\`),
	}
	seen := make(map[string]bool)
	encodings := make([][]byte, 0, len(variants)*2)
	for _, variant := range variants {
		if variant == "" {
			continue
		}
		utf8Bytes := foldASCII([]byte(variant))
		if !seen[string(utf8Bytes)] {
			seen[string(utf8Bytes)] = true
			encodings = append(encodings, utf8Bytes)
		}
		utf16Bytes := foldASCII(utf16LE(variant))
		if !seen[string(utf16Bytes)] {
			seen[string(utf16Bytes)] = true
			encodings = append(encodings, utf16Bytes)
		}
	}
	return sensitiveNeedle{label: label, encodings: encodings}
}

func utf16LE(value string) []byte {
	units := utf16.Encode([]rune(value))
	result := make([]byte, 0, len(units)*2)
	for _, unit := range units {
		result = append(result, byte(unit), byte(unit>>8))
	}
	return result
}

func firstSensitiveMatch(contents []byte, needles []sensitiveNeedle) string {
	foldedContents := foldASCII(contents)
	for _, needle := range needles {
		for _, encoding := range needle.encodings {
			if len(encoding) > 0 && bytes.Contains(foldedContents, encoding) {
				return needle.label
			}
		}
	}
	return ""
}

func foldASCII(value []byte) []byte {
	result := append([]byte(nil), value...)
	for index, item := range result {
		if item >= 'A' && item <= 'Z' {
			result[index] = item + ('a' - 'A')
		}
	}
	return result
}

func environmentValue(environment []string, name string) string {
	for _, entry := range environment {
		key, value, found := strings.Cut(entry, "=")
		if found && strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}

func randomCanary(t *testing.T, role string) string {
	t.Helper()
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		t.Fatal("CANARY_GENERATION_FAILED")
	}
	return "MW_" + strings.ToUpper(role) + "_" + hex.EncodeToString(value)
}

func writeSyntheticArtifact(t *testing.T, name string, contents []byte) builtArtifact {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(contents)
	return builtArtifact{name: name, path: path, size: int64(len(contents)), sha256: hex.EncodeToString(digest[:])}
}

func requireSafeBoundaryError(t *testing.T, err error, forbidden, required string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), required) {
		t.Fatalf("boundary error did not contain stable code %q", required)
	}
	if forbidden != "" && strings.Contains(err.Error(), forbidden) {
		t.Fatal("boundary error disclosed forbidden diagnostic content")
	}
}

const expectedMindweaverHelp = `MindWeaver local workbench (Go rewrite)

Usage:
  mindweaver
  mindweaver serve [-config mindweaver.v1.json] [-vault ./vault] [-no-browser]
  mindweaver version
  mindweaver config init  [-file mindweaver.v1.json] [-vault ./vault]
  mindweaver config check [-file mindweaver.v1.json]
  mindweaver recovery verify  -backup <backup-directory>
  mindweaver recovery restore -backup <backup-directory> -vault <new-vault-directory>

Recovery is a mutually exclusive startup mode. Backups are plaintext, and
restore never overwrites or merges an existing Vault.
`

const expectedPDFProbe = "MWPDF-PROBE/1\nhelper=mindweaver-pdf\nextract=MWPDF1\n"

var errProcessOutputLimit = errors.New("process output limit exceeded")
