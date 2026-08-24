package pdfextract

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
		{name: "invalid", path: invalid, want: ErrInvalidPDF},
		{name: "no extracted text", path: empty, want: ErrNoExtractedText},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := client.Extract(context.Background(), test.path); !errors.Is(err, test.want) {
				t.Fatalf("Extract error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestParseRejectsInvalidAndEmptyPDF(t *testing.T) {
	t.Parallel()
	invalid := filepath.Join(t.TempDir(), "invalid.pdf")
	if err := os.WriteFile(invalid, []byte("not a pdf"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := parseFile(invalid); err == nil {
		t.Fatal("invalid PDF unexpectedly parsed")
	}

	empty := filepath.Join(t.TempDir(), "empty.pdf")
	writeSimplePDF(t, empty, "")
	if _, err := parseFile(empty); err == nil {
		t.Fatal("empty PDF unexpectedly produced text")
	}
}

func TestDecodeResultRejectsMalformedFrames(t *testing.T) {
	t.Parallel()
	for _, frame := range [][]byte{
		[]byte("wrong\npages=1\nbytes=1\nx"),
		[]byte("MWPDF1\npages=0\nbytes=1\nx"),
		[]byte("MWPDF1\npages=1\nbytes=2\nx"),
		[]byte("MWPDF1\npages=1\nbytes=1\n\xff"),
	} {
		if _, err := decodeResult(frame); err == nil {
			t.Fatalf("frame %q unexpectedly accepted", frame)
		}
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
	if err := RunHelper(args, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(HelperExitCode(err))
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
