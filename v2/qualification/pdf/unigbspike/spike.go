// Package unigbspike is qualification-only evidence for one uncompressed
// UniGB-UCS2-H text-layer shape. It is not a general PDF parser and must not be
// imported by production packages.
package unigbspike

import (
	"bytes"
	"encoding/hex"
	"errors"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

var (
	ErrInvalid     = errors.New("qualification unigb spike: invalid input")
	ErrResource    = errors.New("qualification unigb spike: resource limit")
	ErrUnsupported = errors.New("qualification unigb spike: unsupported PDF shape")
)

type Limits struct {
	MaxSourceBytes    int
	MaxExtractedBytes int
	MaxPages          int
	MaxTextOperands   int
}

var DefaultLimits = Limits{
	MaxSourceBytes:    32 << 20,
	MaxExtractedBytes: 32 << 20,
	MaxPages:          2000,
	MaxTextOperands:   8192,
}

// Extract decodes only direct hexadecimal Tj operands in an uncompressed
// UniGB-UCS2-H content stream. This deliberately narrow spike proves a
// decoding mechanism, not production parser fitness.
func Extract(source []byte, limits Limits) (string, error) {
	if limits.MaxSourceBytes < 1 || limits.MaxExtractedBytes < 1 || limits.MaxPages < 1 || limits.MaxTextOperands < 1 {
		return "", ErrResource
	}
	if len(source) < 8 || !bytes.HasPrefix(source, []byte("%PDF-")) {
		return "", ErrInvalid
	}
	if len(source) > limits.MaxSourceBytes {
		return "", ErrResource
	}
	if !bytes.Contains(source, []byte("/Encoding /UniGB-UCS2-H")) {
		return "", ErrUnsupported
	}
	if bytes.Contains(source, []byte("/Filter")) {
		return "", ErrUnsupported
	}
	pages := bytes.Count(source, []byte("/Type /Page "))
	if pages < 1 {
		return "", ErrInvalid
	}
	if pages > limits.MaxPages {
		return "", ErrResource
	}

	lines := make([]string, 0, 16)
	extractedBytes := 0
	for cursor := 0; cursor < len(source); {
		start := bytes.IndexByte(source[cursor:], '<')
		if start < 0 {
			break
		}
		start += cursor
		if start+1 < len(source) && source[start+1] == '<' {
			cursor = start + 2
			continue
		}
		end := bytes.IndexByte(source[start+1:], '>')
		if end < 0 {
			return "", ErrInvalid
		}
		end += start + 1
		next := end + 1
		for next < len(source) && isPDFWhitespace(source[next]) {
			next++
		}
		if next+2 > len(source) || string(source[next:next+2]) != "Tj" {
			cursor = end + 1
			continue
		}
		if len(lines) >= limits.MaxTextOperands {
			return "", ErrResource
		}
		encoded := source[start+1 : end]
		if len(encoded) == 0 || len(encoded)%4 != 0 {
			return "", ErrInvalid
		}
		raw := make([]byte, hex.DecodedLen(len(encoded)))
		if _, err := hex.Decode(raw, encoded); err != nil {
			return "", ErrInvalid
		}
		units := make([]uint16, len(raw)/2)
		for index := range units {
			units[index] = uint16(raw[index*2])<<8 | uint16(raw[index*2+1])
		}
		if !validUTF16(units) {
			return "", ErrInvalid
		}
		line := string(utf16.Decode(units))
		if !utf8.ValidString(line) || strings.IndexByte(line, 0) >= 0 {
			return "", ErrInvalid
		}
		if len(lines) > 0 {
			extractedBytes++
		}
		extractedBytes += len(line)
		if extractedBytes > limits.MaxExtractedBytes {
			return "", ErrResource
		}
		lines = append(lines, line)
		cursor = next + 2
	}
	if len(lines) == 0 {
		return "", ErrUnsupported
	}
	return strings.Join(lines, "\n"), nil
}

func isPDFWhitespace(value byte) bool {
	return value == 0 || value == '\t' || value == '\n' || value == '\f' || value == '\r' || value == ' '
}

func validUTF16(units []uint16) bool {
	for index := 0; index < len(units); index++ {
		switch {
		case 0xD800 <= units[index] && units[index] <= 0xDBFF:
			if index+1 >= len(units) || units[index+1] < 0xDC00 || units[index+1] > 0xDFFF {
				return false
			}
			index++
		case 0xDC00 <= units[index] && units[index] <= 0xDFFF:
			return false
		}
	}
	return true
}
