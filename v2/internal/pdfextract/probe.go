package pdfextract

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
)

const (
	probeCommand          = "-probe"
	probeProtocolMagic    = "MWPDF-PROBE/1"
	maxProbeResponseBytes = 256
	probeResponse         = probeProtocolMagic + "\nhelper=mindweaver-pdf\nextract=" + protocolMagic + "\n"
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
	args := append(append([]string(nil), c.prefixArgs...), probeCommand)
	command := exec.CommandContext(callCtx, c.executable, args...)
	stdout := &limitedBuffer{limit: maxProbeResponseBytes}
	stderr := &limitedBuffer{limit: maxDiagnosticBytes}
	command.Stdout = stdout
	command.Stderr = stderr
	if err := runCommand(command); err != nil {
		if callErr := callCtx.Err(); callErr != nil {
			return fmt.Errorf("%w: %w", ErrHelperUnavailable, callErr)
		}
		// Start/exit errors and stderr may contain attacker-controlled details.
		// Availability needs only the stable category, never those diagnostics.
		return ErrHelperUnavailable
	}
	if stdout.overflow {
		return errors.Join(ErrHelperUnavailable, ErrHelperProtocol)
	}
	if err := decodeProbe(stdout.Bytes()); err != nil {
		return errors.Join(ErrHelperUnavailable, err)
	}
	return nil
}

func decodeProbe(data []byte) error {
	if len(data) > maxProbeResponseBytes || !bytes.Equal(data, []byte(probeResponse)) {
		return ErrHelperProtocol
	}
	return nil
}
