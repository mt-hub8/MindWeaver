// Package client invokes the isolated MindWeaver PDF parser helper.
package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/pdfextract/protocol"
)

const (
	defaultTimeout     = 30 * time.Second
	maxDiagnosticBytes = 4096
)

var (
	ErrHelperFailed      = errors.New("pdfextract: helper failed")
	ErrHelperUnavailable = errors.New("pdfextract: helper capability unavailable")
)

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
func (c *Client) Extract(ctx context.Context, sourcePath string) (protocol.Result, error) {
	if c == nil {
		return protocol.Result{}, errors.New("pdfextract: nil client")
	}
	if ctx == nil {
		return protocol.Result{}, errors.New("pdfextract: nil context")
	}
	absolute, err := filepath.Abs(sourcePath)
	if err != nil {
		return protocol.Result{}, fmt.Errorf("pdfextract: resolve source: %w", err)
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return protocol.Result{}, fmt.Errorf("pdfextract: inspect source: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() < 8 {
		return protocol.Result{}, protocol.ErrInvalidPDF
	}
	if info.Size() > protocol.MaxSourceBytes {
		return protocol.Result{}, protocol.ErrResourceLimit
	}

	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	args := append(append([]string(nil), c.prefixArgs...), "-input", absolute)
	command := exec.CommandContext(callCtx, c.executable, args...)
	stdout := &limitedBuffer{limit: protocol.MaxExtractedTextBytes + 256}
	stderr := &limitedBuffer{limit: maxDiagnosticBytes}
	command.Stdout = stdout
	command.Stderr = stderr
	if err := runCommand(command); err != nil {
		if callCtx.Err() != nil {
			return protocol.Result{}, callCtx.Err()
		}
		if classified := classifyHelperExit(err); classified != nil {
			return protocol.Result{}, classified
		}
		return protocol.Result{}, ErrHelperFailed
	}
	if stdout.overflow {
		return protocol.Result{}, protocol.ErrResourceLimit
	}
	result, err := protocol.DecodeResult(stdout.Bytes())
	if err != nil {
		return protocol.Result{}, err
	}
	return result, nil
}

func classifyHelperExit(err error) error {
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		return nil
	}
	return protocol.ErrorForExitCode(exitError.ExitCode())
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

func (b *limitedBuffer) Bytes() []byte { return b.buffer.Bytes() }
