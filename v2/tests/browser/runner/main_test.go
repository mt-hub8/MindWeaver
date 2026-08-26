package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const runnerTestRevision = "0123456789abcdef0123456789abcdef01234567"

func TestRunWritesNewBlockedContentFreeReport(t *testing.T) {
	root := filepath.Join(t.TempDir(), "vault-path-canary-8a3f")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(root, "ui-browser-report.json")
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"-source-revision", runnerTestRevision, "-report", reportPath}, &stdout, &stderr)
	if exitCode != exitBlocked || stdout.String() != "UI-001/UI-002 BLOCKED BROWSER_ARTIFACT_NOT_APPROVED\n" || stderr.Len() != 0 {
		t.Fatalf("exit/stdout/stderr = %d/%q/%q", exitCode, stdout.String(), stderr.String())
	}
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > 32<<10 || bytes.Contains(data, []byte(root)) || bytes.Contains(data, []byte("vault-path-canary-8a3f")) {
		t.Fatal("report leaked its output path or exceeded the evidence bound")
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if document["status"] != "BLOCKED" || document["code"] != "BROWSER_ARTIFACT_NOT_APPROVED" ||
		document["cleanupStatus"] != "NOT_STARTED" {
		t.Fatalf("report tuple = %#v", document)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "ui-browser-report.json" {
		t.Fatalf("report directory entries = %#v", entries)
	}
}

func TestRunNeverOverwritesEvidence(t *testing.T) {
	root := t.TempDir()
	reportPath := filepath.Join(root, "ui-browser-report.json")
	first := []byte("existing-evidence-canary")
	if err := os.WriteFile(reportPath, first, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"-source-revision", runnerTestRevision, "-report", reportPath}, &stdout, &stderr); exitCode != exitFailed {
		t.Fatalf("overwrite exit code = %d", exitCode)
	}
	if stdout.Len() != 0 || stderr.String() != "browser qualification: report unavailable\n" {
		t.Fatalf("overwrite stdout/stderr = %q/%q", stdout.String(), stderr.String())
	}
	after, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, first) {
		t.Fatalf("existing report changed to %q", after)
	}
}

func TestRunRejectsUnsafeArgumentsWithoutWriting(t *testing.T) {
	for name, arguments := range map[string][]string{
		"short revision":  {"-source-revision", "abc", "-report", filepath.Join(t.TempDir(), "report.json")},
		"relative report": {"-source-revision", runnerTestRevision, "-report", "report.json"},
		"wrong extension": {"-source-revision", runnerTestRevision, "-report", filepath.Join(t.TempDir(), "report.txt")},
		"extra argument":  {"-source-revision", runnerTestRevision, "-report", filepath.Join(t.TempDir(), "report.json"), "extra"},
	} {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if exitCode := run(arguments, &stdout, &stderr); exitCode != exitInvalid {
				t.Fatalf("invalid-argument exit code = %d", exitCode)
			}
			if stdout.Len() != 0 || stderr.String() != "browser qualification: invalid arguments\n" {
				t.Fatalf("invalid-argument stdout/stderr = %q/%q", stdout.String(), stderr.String())
			}
		})
	}
}

func TestRunHidesReportFilesystemFailure(t *testing.T) {
	secretParent := filepath.Join(t.TempDir(), "secret-parent-canary", "missing")
	reportPath := filepath.Join(secretParent, "report.json")
	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"-source-revision", runnerTestRevision, "-report", reportPath}, &stdout, &stderr); exitCode != exitFailed {
		t.Fatalf("filesystem-failure exit code = %d", exitCode)
	}
	if stdout.Len() != 0 || stderr.String() != "browser qualification: report unavailable\n" ||
		strings.Contains(stderr.String(), secretParent) {
		t.Fatalf("filesystem-failure stdout/stderr = %q/%q", stdout.String(), stderr.String())
	}
	if _, err := os.Stat(secretParent); !os.IsNotExist(err) {
		t.Fatalf("runner created rejected report parent: %v", err)
	}
}
