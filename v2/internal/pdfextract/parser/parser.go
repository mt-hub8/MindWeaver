// Package parser implements the state-free PDF helper command. It is the only
// production package allowed to link the PDF parser dependency.
package parser

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	pdf "github.com/ledongthuc/pdf"
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
	result, err := parseFile(args[1])
	if err != nil {
		return err
	}
	return protocol.WriteResult(output, result)
}

func parseFile(path string) (protocol.Result, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 8 {
		return protocol.Result{}, protocol.ErrInvalidPDF
	}
	if info.Size() > protocol.MaxSourceBytes {
		return protocol.Result{}, protocol.ErrResourceLimit
	}
	file, err := os.Open(path)
	if err != nil {
		return protocol.Result{}, fmt.Errorf("pdfextract: open source: %w", err)
	}
	var signature [5]byte
	_, readErr := io.ReadFull(file, signature[:])
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || string(signature[:]) != "%PDF-" {
		return protocol.Result{}, protocol.ErrInvalidPDF
	}

	handle, reader, err := pdf.Open(path)
	if err != nil {
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "encrypt") || strings.Contains(message, "password") {
			return protocol.Result{}, protocol.ErrEncryptedPDF
		}
		return protocol.Result{}, protocol.ErrInvalidPDF
	}
	defer handle.Close()
	pages := reader.NumPage()
	if pages < 1 {
		return protocol.Result{}, protocol.ErrInvalidPDF
	}
	if pages > protocol.MaxPages {
		return protocol.Result{}, protocol.ErrResourceLimit
	}
	plain, err := reader.GetPlainText()
	if err != nil {
		return protocol.Result{}, protocol.ErrInvalidPDF
	}
	data, err := io.ReadAll(io.LimitReader(plain, protocol.MaxExtractedTextBytes+1))
	if err != nil {
		return protocol.Result{}, protocol.ErrInvalidPDF
	}
	if len(data) > protocol.MaxExtractedTextBytes {
		return protocol.Result{}, protocol.ErrResourceLimit
	}
	if !utf8.Valid(data) {
		return protocol.Result{}, protocol.ErrInvalidPDF
	}
	text := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n"))
	if text == "" {
		return protocol.Result{}, protocol.ErrNoExtractedText
	}
	if strings.IndexByte(text, 0) >= 0 {
		return protocol.Result{}, protocol.ErrInvalidPDF
	}
	return protocol.Result{Text: text, Pages: pages}, nil
}
