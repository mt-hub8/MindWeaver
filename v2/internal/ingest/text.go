// Package ingest contains the concrete, deterministic text preparation used by
// the first MindWeaver ingestion worker.
package ingest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// DefaultChunkRunes and DefaultChunkOverlap are the single production
	// chunking profile shared by admission proof and the worker.
	DefaultChunkRunes   = 1200
	DefaultChunkOverlap = 100
	// MaxTextSourceBytes is the public text admission ceiling. With the default
	// profile the smallest possible forward step is 1200/2+1-100 = 501 runes.
	// A worst-case 4 MiB ASCII source therefore creates at most
	// 1+ceil((4194304-1200)/501) = 8371 chunks, below MaxTextChunks. Every valid
	// multibyte UTF-8 source of the same byte size has no more runes, while each
	// 1200-rune chunk is at most 4800 bytes, below SQLite's 64 KiB row bound.
	MaxTextSourceBytes int64 = 4 << 20
	// MaxChunkRunes and MaxTextChunks cap both individual records and total
	// allocation. Together with the overlap ratio they prevent configuration
	// mistakes from amplifying one bounded source into an unbounded chunk set.
	MaxChunkRunes      = 8 << 10
	MaxTextChunks      = 10_000
	textReadBufferSize = 64 << 10
	maxEmptyTextReads  = 100
)

var (
	ErrUnsupportedFormat = errors.New("ingest: unsupported text format")
	ErrSourceTooLarge    = errors.New("ingest: source exceeds byte limit")
	ErrInvalidUTF8       = errors.New("ingest: source is not valid UTF-8")
	ErrBinaryText        = errors.New("ingest: source contains NUL bytes")
	ErrEmptyText         = errors.New("ingest: source contains no searchable text")
	ErrChunkLimit        = errors.New("ingest: chunk configuration exceeds resource limits")
)

// TextFormat is the small set of formats handled without an external parser.
type TextFormat string

const (
	FormatText     TextFormat = "text"
	FormatMarkdown TextFormat = "markdown"
	// FormatPDF is accepted by filename detection but parsed only through the
	// separately isolated pdfextract helper.
	FormatPDF TextFormat = "pdf"
)

// DetectTextFormat accepts only an explicit TXT, Markdown, or PDF filename.
// Callers must route PDF to the separately qualified parser and must never
// silently treat arbitrary PDF bytes as text.
func DetectTextFormat(name string) (TextFormat, error) {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(name))) {
	case ".txt":
		return FormatText, nil
	case ".md", ".markdown":
		return FormatMarkdown, nil
	case ".pdf":
		return FormatPDF, nil
	default:
		return "", ErrUnsupportedFormat
	}
}

// ReadText reads a bounded UTF-8 source. It strips one UTF-8 BOM, normalizes
// line endings to LF, and rejects NUL-containing input rather than indexing a
// likely binary file. The returned text keeps Markdown syntax because it is
// useful lexical content and preserves source fidelity for citations. Context
// is checked around every Read, but Go cannot interrupt a Reader already stuck
// inside its Read method; production ingestion passes a regular local file.
func ReadText(ctx context.Context, src io.Reader, maxBytes int64) (string, error) {
	if ctx == nil {
		return "", errors.New("ingest: nil context")
	}
	if src == nil {
		return "", errors.New("ingest: nil source")
	}
	if maxBytes < 1 || maxBytes > MaxTextSourceBytes {
		return "", fmt.Errorf("ingest: byte limit must be between 1 and %d", MaxTextSourceBytes)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	data, err := readBounded(ctx, src, maxBytes)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(data) >= 3 && data[0] == 0xef && data[1] == 0xbb && data[2] == 0xbf {
		data = data[3:]
	}
	if !utf8.Valid(data) {
		return "", ErrInvalidUTF8
	}
	if strings.IndexByte(string(data), 0) >= 0 {
		return "", ErrBinaryText
	}

	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if strings.TrimSpace(text) == "" {
		return "", ErrEmptyText
	}
	return text, nil
}

func readBounded(ctx context.Context, src io.Reader, maxBytes int64) ([]byte, error) {
	var output bytes.Buffer
	buffer := make([]byte, textReadBufferSize)
	var total int64
	emptyReads := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		readSize := len(buffer)
		remaining := maxBytes - total
		if remaining < int64(readSize) {
			readSize = int(remaining) + 1
		}
		n, readErr := src.Read(buffer[:readSize])
		if n < 0 || n > readSize {
			return nil, fmt.Errorf("ingest: source returned invalid read count %d", n)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if int64(n) > remaining {
			return nil, ErrSourceTooLarge
		}
		if n > 0 {
			emptyReads = 0
			if _, err := output.Write(buffer[:n]); err != nil {
				return nil, fmt.Errorf("ingest: buffer text source: %w", err)
			}
			total += int64(n)
		} else if readErr == nil {
			emptyReads++
			if emptyReads >= maxEmptyTextReads {
				return nil, fmt.Errorf("ingest: read text source: %w", io.ErrNoProgress)
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return output.Bytes(), nil
			}
			return nil, fmt.Errorf("ingest: read text source: %w", readErr)
		}
	}
}

// Chunk is one deterministic lexical unit. Digest is lowercase SHA-256 of the
// exact Text field and allows the database writer to detect accidental drift.
type Chunk struct {
	Ordinal int
	Text    string
	Digest  string
}

// ChunkText creates bounded, overlapping rune-safe chunks. It prefers paragraph,
// line, sentence, then whitespace boundaries in the latter half of the window.
// It never splits a UTF-8 sequence and always makes progress.
func ChunkText(text string, maxRunes, overlapRunes int) ([]Chunk, error) {
	if int64(len(text)) > MaxTextSourceBytes {
		return nil, ErrSourceTooLarge
	}
	if !utf8.ValidString(text) {
		return nil, ErrInvalidUTF8
	}
	if maxRunes < 1 || maxRunes > MaxChunkRunes {
		return nil, fmt.Errorf("%w: max chunk runes must be between 1 and %d", ErrChunkLimit, MaxChunkRunes)
	}
	if overlapRunes < 0 || overlapRunes > maxRunes/4 {
		return nil, fmt.Errorf("%w: overlap must be non-negative and at most one quarter of the chunk", ErrChunkLimit)
	}
	if strings.TrimSpace(text) == "" {
		return nil, ErrEmptyText
	}

	runes := []rune(text)
	// preferredBreak may choose the first boundary just after the midpoint.
	// Calculate against that minimum advance, not the hard-window advance, so
	// preflight cannot underestimate a boundary-heavy source.
	minimumAdvance := maxRunes/2 + 1 - overlapRunes
	worstChunkCount := 1
	if len(runes) > maxRunes {
		worstChunkCount += (len(runes) - maxRunes + minimumAdvance - 1) / minimumAdvance
	}
	if worstChunkCount > MaxTextChunks {
		return nil, fmt.Errorf("%w: source may create %d chunks; maximum is %d", ErrChunkLimit, worstChunkCount, MaxTextChunks)
	}
	chunks := make([]Chunk, 0, worstChunkCount)
	for start := 0; start < len(runes); {
		end := start + maxRunes
		if end >= len(runes) {
			end = len(runes)
		} else {
			end = preferredBreak(runes, start, end)
		}

		value := strings.TrimSpace(string(runes[start:end]))
		if value != "" {
			if len(chunks) == MaxTextChunks {
				return nil, fmt.Errorf("%w: maximum chunk count reached", ErrChunkLimit)
			}
			digest := sha256.Sum256([]byte(value))
			chunks = append(chunks, Chunk{
				Ordinal: len(chunks),
				Text:    value,
				Digest:  hex.EncodeToString(digest[:]),
			})
		}
		if end == len(runes) {
			break
		}
		next := end - overlapRunes
		if next <= start {
			next = end
		}
		start = next
	}
	if len(chunks) == 0 {
		return nil, ErrEmptyText
	}
	return chunks, nil
}

func preferredBreak(runes []rune, start, hardEnd int) int {
	softStart := start + (hardEnd-start)/2
	for _, class := range []func(rune) bool{
		func(r rune) bool { return r == '\n' },
		func(r rune) bool { return strings.ContainsRune("。！？.!?；;", r) },
		unicode.IsSpace,
	} {
		for index := hardEnd - 1; index >= softStart; index-- {
			if class(runes[index]) {
				return index + 1
			}
		}
	}
	return hardEnd
}
