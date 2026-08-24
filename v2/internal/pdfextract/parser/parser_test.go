package parser

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mt-hub8/MindWeaver/v2/internal/pdfextract/protocol"
)

func TestParseRejectsInvalidAndEmptyPDF(t *testing.T) {
	t.Parallel()
	invalid := filepath.Join(t.TempDir(), "invalid.pdf")
	if err := os.WriteFile(invalid, []byte("not a pdf"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := parseFile(invalid); !errors.Is(err, protocol.ErrInvalidPDF) {
		t.Fatalf("invalid PDF error = %v, want ErrInvalidPDF", err)
	}

	empty := filepath.Join(t.TempDir(), "empty.pdf")
	writeSimplePDF(t, empty, "")
	if _, err := parseFile(empty); !errors.Is(err, protocol.ErrNoExtractedText) {
		t.Fatalf("empty PDF error = %v, want ErrNoExtractedText", err)
	}
}

func TestRunProbeIsExactAndDoesNotAcceptAliases(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	if err := Run([]string{protocol.ProbeArgument}, &output); err != nil {
		t.Fatalf("Run probe: %v", err)
	}
	if err := protocol.DecodeProbe(output.Bytes()); err != nil {
		t.Fatalf("probe response: %v", err)
	}

	for _, args := range [][]string{
		nil,
		{"probe"},
		{"--probe"},
		{"-probe=true"},
		{protocol.ProbeArgument, protocol.ProbeArgument},
		{protocol.ProbeArgument, "trailing"},
	} {
		output.Reset()
		if err := Run(args, &output); err == nil || output.Len() != 0 {
			t.Fatalf("Run(%q) = output %q, error %v; want strict rejection", args, output.String(), err)
		}
	}
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
