// Package protocol defines the bounded, versioned wire contract shared by the
// MindWeaver process and its isolated PDF parser helper.
package protocol

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	MaxSourceBytes        int64 = 32 << 20
	MaxExtractedTextBytes       = 32 << 20
	MaxPages                    = 2000
	MaxProbeResponseBytes       = 256
	ProbeArgument               = "-probe"

	extractMagic              = "MWPDF1"
	probeMagic                = "MWPDF-PROBE/1"
	helperExitInvalidPDF      = 20
	helperExitEncryptedPDF    = 21
	helperExitResourceLimit   = 22
	helperExitNoExtractedText = 23
)

var (
	ErrInvalidPDF      = errors.New("pdfextract: invalid text PDF")
	ErrEncryptedPDF    = errors.New("pdfextract: encrypted PDF is not supported")
	ErrResourceLimit   = errors.New("pdfextract: resource limit exceeded")
	ErrHelperProtocol  = errors.New("pdfextract: invalid helper protocol")
	ErrNoExtractedText = errors.New("pdfextract: PDF contains no extractable text")
)

var probeResponse = []byte(probeMagic + "\nhelper=mindweaver-pdf\nextract=" + extractMagic + "\n")

// Result is the bounded, UTF-8 output returned by the isolated parser.
type Result struct {
	Text  string
	Pages int
}

// WriteResult emits exactly one canonical extraction frame.
func WriteResult(output io.Writer, result Result) error {
	if output == nil {
		return errors.New("pdfextract: nil helper output")
	}
	if result.Pages < 1 || result.Pages > MaxPages || len(result.Text) < 1 ||
		len(result.Text) > MaxExtractedTextBytes || !utf8.ValidString(result.Text) ||
		strings.IndexByte(result.Text, 0) >= 0 {
		return ErrHelperProtocol
	}
	if _, err := fmt.Fprintf(output, "%s\npages=%d\nbytes=%d\n", extractMagic, result.Pages, len(result.Text)); err != nil {
		return fmt.Errorf("pdfextract: write header: %w", err)
	}
	if _, err := io.WriteString(output, result.Text); err != nil {
		return fmt.Errorf("pdfextract: write text: %w", err)
	}
	return nil
}

// DecodeResult accepts exactly one canonical, bounded extraction frame.
func DecodeResult(data []byte) (Result, error) {
	reader := bufio.NewReader(bytes.NewReader(data))
	magic, err := readLine(reader)
	if err != nil || magic != extractMagic {
		return Result{}, ErrHelperProtocol
	}
	pageLine, err := readLine(reader)
	if err != nil || !strings.HasPrefix(pageLine, "pages=") {
		return Result{}, ErrHelperProtocol
	}
	pages, err := strconv.Atoi(strings.TrimPrefix(pageLine, "pages="))
	if err != nil || pages < 1 || pages > MaxPages {
		return Result{}, ErrHelperProtocol
	}
	byteLine, err := readLine(reader)
	if err != nil || !strings.HasPrefix(byteLine, "bytes=") {
		return Result{}, ErrHelperProtocol
	}
	length, err := strconv.Atoi(strings.TrimPrefix(byteLine, "bytes="))
	if err != nil || length < 1 || length > MaxExtractedTextBytes {
		return Result{}, ErrHelperProtocol
	}
	text, err := io.ReadAll(io.LimitReader(reader, int64(MaxExtractedTextBytes)+1))
	if err != nil || len(text) != length || reader.Buffered() != 0 || !utf8.Valid(text) || bytes.IndexByte(text, 0) >= 0 {
		return Result{}, ErrHelperProtocol
	}
	return Result{Text: string(text), Pages: pages}, nil
}

// WriteProbe emits the one accepted helper capability frame.
func WriteProbe(output io.Writer) error {
	if output == nil {
		return errors.New("pdfextract: nil helper output")
	}
	if _, err := output.Write(probeResponse); err != nil {
		return fmt.Errorf("pdfextract: write probe: %w", err)
	}
	return nil
}

// DecodeProbe requires the exact versioned capability frame.
func DecodeProbe(data []byte) error {
	if len(data) > MaxProbeResponseBytes || !bytes.Equal(data, probeResponse) {
		return ErrHelperProtocol
	}
	return nil
}

// ExitCode maps controlled parser failures to stable process exit codes.
func ExitCode(err error) int {
	switch {
	case errors.Is(err, ErrInvalidPDF):
		return helperExitInvalidPDF
	case errors.Is(err, ErrEncryptedPDF):
		return helperExitEncryptedPDF
	case errors.Is(err, ErrResourceLimit):
		return helperExitResourceLimit
	case errors.Is(err, ErrNoExtractedText):
		return helperExitNoExtractedText
	default:
		return 1
	}
}

// ErrorForExitCode decodes only controlled parser exit codes. Unknown codes
// intentionally remain unclassified so callers can collapse them safely.
func ErrorForExitCode(code int) error {
	switch code {
	case helperExitInvalidPDF:
		return ErrInvalidPDF
	case helperExitEncryptedPDF:
		return ErrEncryptedPDF
	case helperExitResourceLimit:
		return ErrResourceLimit
	case helperExitNoExtractedText:
		return ErrNoExtractedText
	default:
		return nil
	}
}

func readLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil || len(line) > 64 {
		return "", ErrHelperProtocol
	}
	return strings.TrimSuffix(line, "\n"), nil
}
