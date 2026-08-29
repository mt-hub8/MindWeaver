// Package parser implements the state-free PDF helper command. It is the only
// production package allowed to link the PDF parser dependency.
package parser

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/mt-hub8/MindWeaver/v2/internal/pdfextract/protocol"
)

// Run is the complete implementation of the parser helper command.
func Run(args []string, output io.Writer) (err error) {
	defer func() {
		if recover() != nil {
			err = protocol.ErrInvalidPDF
		}
	}()
	if output == nil {
		return errors.New("pdfextract: nil helper output")
	}
	if len(args) == 1 && args[0] == protocol.ProbeArgument {
		return protocol.WriteProbe(output)
	}
	if len(args) != 2 || args[0] != "-input" || strings.TrimSpace(args[1]) == "" {
		return errors.New("usage: mindweaver-pdf -input <content-addressed-file>")
	}
	result, err := parseFileContext(context.Background(), args[1])
	if err != nil {
		return err
	}
	return protocol.WriteResult(output, result)
}

func parseFile(path string) (protocol.Result, error) {
	return parseFileContext(context.Background(), path)
}
