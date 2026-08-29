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

	"github.com/mt-hub8/MindWeaver/v2/internal/pdfextract/parser"
	"github.com/mt-hub8/MindWeaver/v2/internal/pdfextract/protocol"
)

func TestClientExtractsTextThroughHelperProcess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.pdf")
	writeSimplePDF(t, path, "Mind Weaver PDF searchable context")
	t.Setenv("GO_WANT_PDF_HELPER", "1")
	client, err := newClient(os.Args[0], []string{"-test.run=TestPDFHelperProcess", "--"}, 5*time.Second)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	result, err := client.Extract(context.Background(), path)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if result.Pages != 1 || !strings.Contains(result.Text, "Mind Weaver PDF searchable context") {
		t.Fatalf("result = %#v", result)
	}
}

func TestClientPreservesControlledHelperFailureCategories(t *testing.T) {
	t.Setenv("GO_WANT_PDF_HELPER", "1")
	client, err := newClient(os.Args[0], []string{"-test.run=TestPDFHelperProcess", "--"}, 5*time.Second)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	invalid := filepath.Join(t.TempDir(), "invalid.pdf")
	if err := os.WriteFile(invalid, []byte("not a pdf"), 0o600); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(t.TempDir(), "empty.pdf")
	writeSimplePDF(t, empty, "")
	for _, test := range []struct {
		name string
		path string
		want error
	}{
		{name: "invalid", path: invalid, want: protocol.ErrInvalidPDF},
		{name: "no extracted text", path: empty, want: protocol.ErrNoExtractedText},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := client.Extract(context.Background(), test.path); !errors.Is(err, test.want) {
				t.Fatalf("Extract error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestClientExtractCancellationReapsHelperAndPreventsSentinel(t *testing.T) {
	client := newPDFHelperProcessClient(t, 5*time.Second)
	root := t.TempDir()
	source := filepath.Join(root, "source.pdf")
	writeSimplePDF(t, source, "cancel a real extract subprocess")
	marker := filepath.Join(root, "started")
	sentinel := filepath.Join(root, "survived")
	const sentinelDelay = 750 * time.Millisecond
	t.Setenv(pdfHelperTestModeEnv, "extract-sentinel")
	t.Setenv(pdfHelperTestMarkerEnv, marker)
	t.Setenv(pdfHelperTestSentinelEnv, sentinel)
	t.Setenv(pdfHelperTestSentinelDelayEnv, sentinelDelay.String())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := client.Extract(ctx, source)
		done <- err
	}()
	waitForHelperMarker(t, marker)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Extract cancellation error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled Extract did not reap its helper")
	}

	// Extract returning is the client boundary's reap point. Waiting past the
	// helper's sentinel deadline proves the cancelled process did not survive it.
	time.Sleep(sentinelDelay + 250*time.Millisecond)
	if _, err := os.Lstat(sentinel); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled Extract helper survived: sentinel error = %v", err)
	}
}

func TestBundledHelperFailureChannelIsEmpty(t *testing.T) {
	helper := buildBundledHelper(t, runtime.GOOS, runtime.GOARCH)
	root := t.TempDir()
	pathCanary := "PATH_CANARY_CREDENTIAL_DO_NOT_LEAK"
	contentCanary := "CONTENT_CANARY_PROMPT_DO_NOT_LEAK"
	invalid := filepath.Join(root, pathCanary+".pdf")
	if err := os.WriteFile(invalid, []byte(contentCanary), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(root, pathCanary+"_MISSING.pdf")

	for _, test := range []struct {
		name string
		args []string
		code int
	}{
		{name: "invalid content", args: []string{"-input", invalid}, code: 20},
		{name: "missing path", args: []string{"-input", missing}, code: 20},
		{name: "invalid usage", code: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := exec.Command(helper, test.args...)
			var stdout, stderr bytes.Buffer
			command.Stdout = &stdout
			command.Stderr = &stderr
			err := command.Run()
			var exitError *exec.ExitError
			if !errors.As(err, &exitError) || exitError.ExitCode() != test.code {
				t.Fatalf("helper error = %v, want exit %d", err, test.code)
			}
			if stdout.Len() != 0 || stderr.Len() != 0 {
				t.Fatalf("failure channel was not empty: stdout=%q stderr=%q", stdout.Bytes(), stderr.Bytes())
			}
			for _, canary := range []string{pathCanary, contentCanary} {
				if strings.Contains(stdout.String(), canary) || strings.Contains(stderr.String(), canary) {
					t.Fatalf("helper leaked canary %q", canary)
				}
			}
		})
	}

	client, err := New(helper, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Extract(t.Context(), invalid); !errors.Is(err, protocol.ErrInvalidPDF) ||
		strings.Contains(err.Error(), pathCanary) || strings.Contains(err.Error(), contentCanary) {
		t.Fatalf("client did not preserve the stable exit category: %v", err)
	}
}

func TestPDFHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_PDF_HELPER") != "1" {
		return
	}
	if mode := os.Getenv(pdfHelperTestModeEnv); mode != "" {
		runPDFHelperTestMode(mode)
	}
	args := os.Args
	for index, arg := range args {
		if arg == "--" {
			args = args[index+1:]
			break
		}
	}
	if err := parser.Run(args, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(protocol.ExitCode(err))
	}
	os.Exit(0)
}

func writeSimplePDF(t *testing.T, path, text string) {
	t.Helper()
	escaped := strings.NewReplacer("\\", "\\\\", "(", "\\(", ")", "\\)").Replace(text)
	stream := "BT /F1 12 Tf 72 720 Td (" + escaped + ") Tj ET"
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var output bytes.Buffer
	output.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects)+1)
	for index, object := range objects {
		offsets[index+1] = output.Len()
		fmt.Fprintf(&output, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	xref := output.Len()
	fmt.Fprintf(&output, "xref\n0 %d\n", len(objects)+1)
	output.WriteString("0000000000 65535 f \n")
	for index := 1; index < len(offsets); index++ {
		fmt.Fprintf(&output, "%010d 00000 n \n", offsets[index])
	}
	fmt.Fprintf(&output, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	if err := os.WriteFile(path, output.Bytes(), 0o600); err != nil {
		t.Fatalf("write PDF: %v", err)
	}
}
