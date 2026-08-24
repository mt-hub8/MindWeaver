// Package pdfextract isolates text-PDF parsing behind a bounded helper process.
package pdfextract

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	pdf "github.com/ledongthuc/pdf"
)

const (
	MaxSourceBytes            int64 = 32 << 20
	MaxExtractedTextBytes           = 32 << 20
	MaxPages                        = 2000
	protocolMagic                   = "MWPDF1"
	defaultTimeout                  = 30 * time.Second
	maxDiagnosticBytes              = 4096
	helperExitInvalidPDF            = 20
	helperExitEncryptedPDF          = 21
	helperExitResourceLimit         = 22
	helperExitNoExtractedText       = 23
)

var (
	ErrInvalidPDF      = errors.New("pdfextract: invalid text PDF")
	ErrEncryptedPDF    = errors.New("pdfextract: encrypted PDF is not supported")
	ErrResourceLimit   = errors.New("pdfextract: resource limit exceeded")
	ErrHelperProtocol  = errors.New("pdfextract: invalid helper protocol")
	ErrHelperFailed    = errors.New("pdfextract: helper failed")
	ErrNoExtractedText = errors.New("pdfextract: PDF contains no extractable text")
)

// Result is the bounded, UTF-8 output returned by the isolated parser.
type Result struct {
	Text  string
	Pages int
}

// Client invokes one explicit helper executable. The helper does not open a
// Vault or database and can be killed without leaving application state.
type Client struct {
	executable string
	prefixArgs []string
	timeout    time.Duration
}

func New(executable string, timeout time.Duration) (*Client, error) {
	return newClient(executable, nil, timeout)
}

func newClient(executable string, prefixArgs []string, timeout time.Duration) (*Client, error) {
	executable = strings.TrimSpace(executable)
	if executable == "" {
		return nil, errors.New("pdfextract: helper executable is required")
	}
	absolute, err := filepath.Abs(executable)
	if err != nil {
		return nil, fmt.Errorf("pdfextract: resolve helper: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("pdfextract: inspect helper: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("pdfextract: helper is not a regular file")
	}
	if timeout == 0 {
		timeout = defaultTimeout
	}
	if timeout < time.Second || timeout > 2*time.Minute {
		return nil, errors.New("pdfextract: timeout must be between 1s and 2m")
	}
	return &Client{executable: absolute, prefixArgs: append([]string(nil), prefixArgs...), timeout: timeout}, nil
}

// Extract parses a content-addressed source in a separate process. Stdout and
// stderr are bounded before the process starts, and cancellation kills it.
func (c *Client) Extract(ctx context.Context, sourcePath string) (Result, error) {
	if c == nil {
		return Result{}, errors.New("pdfextract: nil client")
	}
	if ctx == nil {
		return Result{}, errors.New("pdfextract: nil context")
	}
	absolute, err := filepath.Abs(sourcePath)
	if err != nil {
		return Result{}, fmt.Errorf("pdfextract: resolve source: %w", err)
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return Result{}, fmt.Errorf("pdfextract: inspect source: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() < 8 {
		return Result{}, ErrInvalidPDF
	}
	if info.Size() > MaxSourceBytes {
		return Result{}, ErrResourceLimit
	}

	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	args := append(append([]string(nil), c.prefixArgs...), "-input", absolute)
	command := exec.CommandContext(callCtx, c.executable, args...)
	stdout := &limitedBuffer{limit: MaxExtractedTextBytes + 256}
	stderr := &limitedBuffer{limit: maxDiagnosticBytes}
	command.Stdout = stdout
	command.Stderr = stderr
	if err := runCommand(command); err != nil {
		if callCtx.Err() != nil {
			return Result{}, callCtx.Err()
		}
		if classified := classifyHelperExit(err); classified != nil {
			return Result{}, classified
		}
		return Result{}, fmt.Errorf("%w: %s", ErrHelperFailed, safeDiagnostic(stderr.String()))
	}
	if stdout.overflow {
		return Result{}, ErrResourceLimit
	}
	result, err := decodeResult(stdout.Bytes())
	if err != nil {
		return Result{}, err
	}
	return result, nil
}

// HelperExitCode is the stable, non-secret process boundary used by the
// bundled helper to preserve controlled parser categories without parsing
// stderr text. Unknown failures intentionally collapse to the generic exit
// code; stderr remains diagnostic-only.
func HelperExitCode(err error) int {
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

func classifyHelperExit(err error) error {
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		return nil
	}
	switch exitError.ExitCode() {
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

// RunHelper is the complete implementation of the parser helper command.
func RunHelper(args []string, output io.Writer) (err error) {
	defer func() {
		if recover() != nil {
			err = ErrInvalidPDF
		}
	}()
	if output == nil {
		return errors.New("pdfextract: nil helper output")
	}
	if len(args) != 2 || args[0] != "-input" || strings.TrimSpace(args[1]) == "" {
		return errors.New("usage: mindweaver-pdf -input <content-addressed-file>")
	}
	result, err := parseFile(args[1])
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(output, "%s\npages=%d\nbytes=%d\n", protocolMagic, result.Pages, len(result.Text)); err != nil {
		return fmt.Errorf("pdfextract: write header: %w", err)
	}
	if _, err := io.WriteString(output, result.Text); err != nil {
		return fmt.Errorf("pdfextract: write text: %w", err)
	}
	return nil
}

func parseFile(path string) (Result, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 8 {
		return Result{}, ErrInvalidPDF
	}
	if info.Size() > MaxSourceBytes {
		return Result{}, ErrResourceLimit
	}
	file, err := os.Open(path)
	if err != nil {
		return Result{}, fmt.Errorf("pdfextract: open source: %w", err)
	}
	var signature [5]byte
	_, readErr := io.ReadFull(file, signature[:])
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || string(signature[:]) != "%PDF-" {
		return Result{}, ErrInvalidPDF
	}

	handle, reader, err := pdf.Open(path)
	if err != nil {
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "encrypt") || strings.Contains(message, "password") {
			return Result{}, ErrEncryptedPDF
		}
		return Result{}, ErrInvalidPDF
	}
	defer handle.Close()
	pages := reader.NumPage()
	if pages < 1 {
		return Result{}, ErrInvalidPDF
	}
	if pages > MaxPages {
		return Result{}, ErrResourceLimit
	}
	plain, err := reader.GetPlainText()
	if err != nil {
		return Result{}, ErrInvalidPDF
	}
	data, err := io.ReadAll(io.LimitReader(plain, MaxExtractedTextBytes+1))
	if err != nil {
		return Result{}, ErrInvalidPDF
	}
	if len(data) > MaxExtractedTextBytes {
		return Result{}, ErrResourceLimit
	}
	if !utf8.Valid(data) {
		return Result{}, ErrInvalidPDF
	}
	text := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n"))
	if text == "" {
		return Result{}, ErrNoExtractedText
	}
	if strings.IndexByte(text, 0) >= 0 {
		return Result{}, ErrInvalidPDF
	}
	return Result{Text: text, Pages: pages}, nil
}

func decodeResult(data []byte) (Result, error) {
	reader := bufio.NewReader(bytes.NewReader(data))
	magic, err := readProtocolLine(reader)
	if err != nil || magic != protocolMagic {
		return Result{}, ErrHelperProtocol
	}
	pageLine, err := readProtocolLine(reader)
	if err != nil || !strings.HasPrefix(pageLine, "pages=") {
		return Result{}, ErrHelperProtocol
	}
	pages, err := strconv.Atoi(strings.TrimPrefix(pageLine, "pages="))
	if err != nil || pages < 1 || pages > MaxPages {
		return Result{}, ErrHelperProtocol
	}
	byteLine, err := readProtocolLine(reader)
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

func readProtocolLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil || len(line) > 64 {
		return "", ErrHelperProtocol
	}
	return strings.TrimSuffix(line, "\n"), nil
}

func safeDiagnostic(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "helper exited without a diagnostic"
	}
	for _, character := range value {
		if character < 0x20 && character != '\t' && character != '\n' && character != '\r' {
			return "helper returned an invalid diagnostic"
		}
	}
	return value
}

type limitedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	if b.overflow {
		return len(data), nil
	}
	remaining := b.limit - b.buffer.Len()
	if remaining <= 0 {
		b.overflow = true
		return len(data), nil
	}
	if len(data) > remaining {
		_, _ = b.buffer.Write(data[:remaining])
		b.overflow = true
		return len(data), nil
	}
	_, _ = b.buffer.Write(data)
	return len(data), nil
}

func (b *limitedBuffer) Bytes() []byte  { return b.buffer.Bytes() }
func (b *limitedBuffer) String() string { return b.buffer.String() }
