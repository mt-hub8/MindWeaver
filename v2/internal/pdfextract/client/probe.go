package client

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	"github.com/mt-hub8/MindWeaver/v2/internal/pdfextract/protocol"
)

// Probe proves that the configured executable is this client's compatible
// mindweaver-pdf helper. It starts the helper through the same platform process
// boundary as Extract and accepts only one fixed, versioned capability frame.
func (c *Client) Probe(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("%w: nil client", ErrHelperUnavailable)
	}
	if ctx == nil {
		return errors.New("pdfextract: nil context")
	}

	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	args := append(append([]string(nil), c.prefixArgs...), protocol.ProbeArgument)
	command := exec.CommandContext(callCtx, c.executable, args...)
	stdout := &limitedBuffer{limit: protocol.MaxProbeResponseBytes}
	stderr := &limitedBuffer{limit: maxDiagnosticBytes}
	command.Stdout = stdout
	command.Stderr = stderr
	if err := runCommand(command); err != nil {
		if callErr := callCtx.Err(); callErr != nil {
			return fmt.Errorf("%w: %w", ErrHelperUnavailable, callErr)
		}
		return ErrHelperUnavailable
	}
	if stdout.overflow {
		return errors.Join(ErrHelperUnavailable, protocol.ErrHelperProtocol)
	}
	if err := protocol.DecodeProbe(stdout.Bytes()); err != nil {
		return errors.Join(ErrHelperUnavailable, err)
	}
	return nil
}
