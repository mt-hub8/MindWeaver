package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/pdfextract/protocol"
)

const (
	pdfHelperTestModeEnv          = "MW_PDF_HELPER_TEST_MODE"
	pdfHelperTestMarkerEnv        = "MW_PDF_HELPER_TEST_MARKER"
	pdfHelperTestSentinelEnv      = "MW_PDF_HELPER_TEST_SENTINEL"
	pdfHelperTestSentinelDelayEnv = "MW_PDF_HELPER_TEST_SENTINEL_DELAY_MS"
	hostileHelperOutput           = "HOSTILE_HELPER_OUTPUT_DO_NOT_LEAK"
)

var errNestedGoUnavailable = errors.New("nested Go tool is unavailable")

type nestedGoSources struct {
	configured string
	lookPath   func(string) (string, error)
	goroot     string
}

func TestClientProbeStartsCompatibleHelperProcess(t *testing.T) {
	client := newPDFHelperProcessClient(t, 5*time.Second)
	if err := client.Probe(t.Context()); err != nil {
		t.Fatalf("Probe: %v", err)
	}
}

func TestBundledHelperExecutablePassesProbe(t *testing.T) {
	helper := buildBundledHelper(t, runtime.GOOS, runtime.GOARCH)
	client, err := New(helper, 5*time.Second)
	if err != nil {
		t.Fatalf("New bundled helper: %v", err)
	}
	if err := client.Probe(t.Context()); err != nil {
		t.Fatalf("bundled helper Probe: %v", err)
	}
}

func TestProbeRejectsWrongExecutableFormats(t *testing.T) {
	corrupt := filepath.Join(t.TempDir(), "not-mindweaver-pdf"+executableSuffix(runtime.GOOS))
	if err := os.WriteFile(corrupt, []byte(hostileHelperOutput), 0o700); err != nil {
		t.Fatal(err)
	}
	assertProbeUnavailableWithoutLeak(t, corrupt)

	wrongOS := "windows"
	if runtime.GOOS == "windows" {
		wrongOS = "linux"
	}
	incompatible := buildBundledHelper(t, wrongOS, runtime.GOARCH)
	assertProbeUnavailableWithoutLeak(t, incompatible)

	if runtime.GOOS == "windows" && runtime.GOARCH == "amd64" {
		wrongArchitecture := buildBundledHelper(t, runtime.GOOS, "arm64")
		assertProbeUnavailableWithoutLeak(t, wrongArchitecture)
	}
}

func TestProbeRejectsNonExecutableHelper(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows executable permission is represented by ACLs and file format")
	}
	helper := filepath.Join(t.TempDir(), "not-executable")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\nexit 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertProbeUnavailableWithoutLeak(t, helper)
}

func TestLocateNestedGoToolUsesStrictPriority(t *testing.T) {
	configured := writeFakeGoTool(t, filepath.Join(t.TempDir(), "configured"))
	pathTool := writeFakeGoTool(t, filepath.Join(t.TempDir(), "path"))
	goroot := t.TempDir()
	writeFakeGoTool(t, filepath.Join(goroot, "bin"))

	lookedUp := false
	got, err := locateNestedGoTool(nestedGoSources{
		configured: configured,
		lookPath: func(string) (string, error) {
			lookedUp = true
			return pathTool, nil
		},
		goroot: goroot,
	})
	if err != nil {
		t.Fatalf("locate configured Go: %v", err)
	}
	if got != configured || lookedUp {
		t.Fatalf("located %q (PATH consulted = %t), want configured %q without PATH lookup", got, lookedUp, configured)
	}

	got, err = locateNestedGoTool(nestedGoSources{
		lookPath: func(name string) (string, error) {
			if name != "go" {
				t.Fatalf("LookPath name = %q, want go", name)
			}
			return pathTool, nil
		},
		goroot: goroot,
	})
	if err != nil || got != pathTool {
		t.Fatalf("PATH Go = %q, %v; want %q", got, err, pathTool)
	}

	got, err = locateNestedGoTool(nestedGoSources{
		lookPath: func(string) (string, error) { return "", exec.ErrNotFound },
		goroot:   goroot,
	})
	wantGOROOT := filepath.Join(goroot, "bin", "go"+executableSuffix(runtime.GOOS))
	if err != nil || got != wantGOROOT {
		t.Fatalf("GOROOT Go = %q, %v; want %q", got, err, wantGOROOT)
	}
}

func TestLocateNestedGoToolFallsBackFromInvalidConfiguredPath(t *testing.T) {
	fallback := writeFakeGoTool(t, filepath.Join(t.TempDir(), "fallback"))
	got, err := locateNestedGoTool(nestedGoSources{
		configured: "relative-go",
		lookPath:   func(string) (string, error) { return fallback, nil },
		goroot:     filepath.Dir(filepath.Dir(fallback)),
	})
	if err != nil || got != fallback {
		t.Fatalf("fallback after invalid MW_GO = %q, %v; want %q", got, err, fallback)
	}
}

func TestLocateNestedGoToolReportsUnavailable(t *testing.T) {
	missingRoot := t.TempDir()
	_, err := locateNestedGoTool(nestedGoSources{
		lookPath: func(string) (string, error) { return "", exec.ErrNotFound },
		goroot:   missingRoot,
	})
	if !errors.Is(err, errNestedGoUnavailable) {
		t.Fatalf("missing nested Go error = %v, want errNestedGoUnavailable", err)
	}
}

func TestClientProbeRejectsHostileProcessFrames(t *testing.T) {
	for _, mode := range []string{"probe-alias", "probe-duplicate", "probe-trailing", "probe-over-limit"} {
		t.Run(mode, func(t *testing.T) {
			client := newPDFHelperProcessClient(t, 5*time.Second)
			t.Setenv(pdfHelperTestModeEnv, mode)
			err := client.Probe(t.Context())
			if !errors.Is(err, ErrHelperUnavailable) || !errors.Is(err, protocol.ErrHelperProtocol) {
				t.Fatalf("Probe error = %v, want unavailable protocol failure", err)
			}
		})
	}
}

func TestHelperErrorsDoNotExposeStderr(t *testing.T) {
	client := newPDFHelperProcessClient(t, 5*time.Second)
	t.Setenv(pdfHelperTestModeEnv, "fail-secret")
	if err := client.Probe(t.Context()); !errors.Is(err, ErrHelperUnavailable) || strings.Contains(err.Error(), hostileHelperOutput) {
		t.Fatalf("Probe error leaked helper output: %v", err)
	}

	path := filepath.Join(t.TempDir(), "source.pdf")
	writeSimplePDF(t, path, "safe source")
	if _, err := client.Extract(t.Context(), path); !errors.Is(err, ErrHelperFailed) || strings.Contains(err.Error(), hostileHelperOutput) {
		t.Fatalf("Extract error leaked helper output: %v", err)
	}
}

func TestClientProbeCancellationKillsRunningHelper(t *testing.T) {
	client := newPDFHelperProcessClient(t, 5*time.Second)
	marker := filepath.Join(t.TempDir(), "started")
	t.Setenv(pdfHelperTestModeEnv, "sleep")
	t.Setenv(pdfHelperTestMarkerEnv, marker)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- client.Probe(ctx) }()
	waitForHelperMarker(t, marker)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || !errors.Is(err, ErrHelperUnavailable) {
			t.Fatalf("Probe cancellation error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled Probe did not terminate its helper")
	}
}

func TestClientProbeTimeoutKillsRunningHelper(t *testing.T) {
	client := newPDFHelperProcessClient(t, 5*time.Second)
	client.timeout = 500 * time.Millisecond
	t.Setenv(pdfHelperTestModeEnv, "sleep")
	started := time.Now()
	err := client.Probe(t.Context())
	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, ErrHelperUnavailable) {
		t.Fatalf("Probe timeout error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("Probe timeout took %v", elapsed)
	}
}

func TestClientProbeRejectsNilInputs(t *testing.T) {
	t.Parallel()
	var client *Client
	if err := client.Probe(t.Context()); !errors.Is(err, ErrHelperUnavailable) {
		t.Fatalf("nil client Probe error = %v", err)
	}
	client = &Client{timeout: time.Second}
	if err := client.Probe(nil); err == nil {
		t.Fatal("Probe(nil) succeeded")
	}
}

func newPDFHelperProcessClient(t *testing.T, timeout time.Duration) *Client {
	t.Helper()
	t.Setenv("GO_WANT_PDF_HELPER", "1")
	client, err := newClient(os.Args[0], []string{"-test.run=^TestPDFHelperProcess$", "--"}, timeout)
	if err != nil {
		t.Fatalf("new helper process client: %v", err)
	}
	return client
}

func runPDFHelperTestMode(mode string) {
	switch mode {
	case "probe-alias":
		_, _ = os.Stdout.WriteString("MWPDF-PROBE/01\nhelper=mindweaver-pdf\nextract=MWPDF1\n")
	case "probe-duplicate":
		_, _ = os.Stdout.WriteString("MWPDF-PROBE/1\nhelper=mindweaver-pdf\nhelper=mindweaver-pdf\nextract=MWPDF1\n")
	case "probe-trailing":
		_, _ = os.Stdout.WriteString(canonicalProbeResponse() + "trailing")
	case "probe-over-limit":
		_, _ = os.Stdout.WriteString(strings.Repeat("x", protocol.MaxProbeResponseBytes+1))
	case "fail-secret":
		_, _ = os.Stderr.WriteString(hostileHelperOutput)
		os.Exit(1)
	case "sleep":
		if marker := os.Getenv(pdfHelperTestMarkerEnv); marker != "" {
			_ = os.WriteFile(marker, []byte("started"), 0o600)
		}
		time.Sleep(30 * time.Second)
	case "extract-sentinel":
		marker := os.Getenv(pdfHelperTestMarkerEnv)
		sentinel := os.Getenv(pdfHelperTestSentinelEnv)
		delay, err := time.ParseDuration(os.Getenv(pdfHelperTestSentinelDelayEnv))
		if marker == "" || sentinel == "" || err != nil || delay <= 0 {
			os.Exit(64)
		}
		if err := os.WriteFile(marker, []byte("started"), 0o600); err != nil {
			os.Exit(74)
		}
		time.Sleep(delay)
		_ = os.WriteFile(sentinel, []byte("survived"), 0o600)
		time.Sleep(30 * time.Second)
	default:
		os.Exit(97)
	}
	os.Exit(0)
}

func canonicalProbeResponse() string {
	var output bytes.Buffer
	if err := protocol.WriteProbe(&output); err != nil {
		panic(err)
	}
	return output.String()
}

func assertProbeUnavailableWithoutLeak(t *testing.T, path string) {
	t.Helper()
	client, err := New(path, 2*time.Second)
	if err != nil {
		t.Fatalf("New wrong helper: %v", err)
	}
	err = client.Probe(t.Context())
	if !errors.Is(err, ErrHelperUnavailable) {
		t.Fatalf("wrong helper Probe error = %v, want ErrHelperUnavailable", err)
	}
	if strings.Contains(err.Error(), hostileHelperOutput) {
		t.Fatalf("wrong helper error leaked file contents: %v", err)
	}
}

func buildBundledHelper(t *testing.T, targetGOOS, targetGOARCH string) string {
	t.Helper()
	suffix := executableSuffix(runtime.GOOS)
	output := filepath.Join(t.TempDir(), "mindweaver-pdf"+suffix)
	goTool, err := locateNestedGoTool(nestedGoSources{
		configured: os.Getenv("MW_GO"),
		lookPath:   exec.LookPath,
		goroot:     runtime.GOROOT(),
	})
	if errors.Is(err, errNestedGoUnavailable) {
		t.Skip("nested Go tool unavailable; skipping helper build")
	}
	if err != nil {
		t.Fatalf("locate nested Go tool: %v", err)
	}
	command := exec.Command(goTool, "build", "-o", output, "./cmd/mindweaver-pdf")
	command.Dir = filepath.Clean(filepath.Join("..", "..", ".."))
	command.Env = hermeticBuildEnvironment(targetGOOS, targetGOARCH)
	buildOutput, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("build helper for %s/%s: %v\n%s", targetGOOS, targetGOARCH, err, buildOutput)
	}
	return output
}

func locateNestedGoTool(sources nestedGoSources) (string, error) {
	if sources.configured != "" {
		if goTool, err := validateNestedGoTool(sources.configured); err == nil {
			return goTool, nil
		}
	}
	if sources.lookPath != nil {
		if candidate, err := sources.lookPath("go"); err == nil {
			absolute, absoluteErr := filepath.Abs(candidate)
			if absoluteErr == nil {
				if goTool, validateErr := validateNestedGoTool(absolute); validateErr == nil {
					return goTool, nil
				}
			}
		}
	}
	if sources.goroot != "" {
		candidate := filepath.Join(sources.goroot, "bin", "go"+executableSuffix(runtime.GOOS))
		if goTool, err := validateNestedGoTool(candidate); err == nil {
			return goTool, nil
		}
	}
	return "", errNestedGoUnavailable
}

func validateNestedGoTool(path string) (string, error) {
	if strings.TrimSpace(path) != path || !filepath.IsAbs(path) {
		return "", errors.New("path must be absolute without surrounding whitespace")
	}
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("inspect executable: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("path is not a regular file")
	}
	if _, err := exec.LookPath(path); err != nil {
		return "", fmt.Errorf("path is not executable: %w", err)
	}
	return path, nil
}

func writeFakeGoTool(t *testing.T, directory string) string {
	t.Helper()
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "go"+executableSuffix(runtime.GOOS))
	if err := os.WriteFile(path, []byte("fake Go tool"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func executableSuffix(goos string) string {
	if goos == "windows" {
		return ".exe"
	}
	return ""
}

func waitForHelperMarker(t *testing.T, marker string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("helper process did not start")
}
