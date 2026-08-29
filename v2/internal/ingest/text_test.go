package ingest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
	"unicode/utf8"
)

func FuzzChunkTextDeterministicAndBounded(f *testing.F) {
	f.Add([]byte("第一段。\n\nSecond paragraph."), uint16(12), uint8(2))
	f.Add([]byte("潮汐校准器\r\nalpha beta gamma"), uint16(8), uint8(1))
	f.Add([]byte{0xff, 0xfe, 0xfd}, uint16(4), uint8(0))
	f.Add([]byte("   "), uint16(1), uint8(0))
	f.Fuzz(func(t *testing.T, data []byte, maxSeed uint16, overlapSeed uint8) {
		// Exact 4 MiB boundary behavior is covered by DOC-001. Keep each fuzz
		// iteration small enough for sustained local and CI fuzzing.
		if len(data) > 64<<10 {
			t.Skip()
		}
		maxRunes := int(maxSeed%512) + 1
		overlapRunes := int(overlapSeed) % (maxRunes/4 + 1)
		first, firstErr := ChunkText(string(data), maxRunes, overlapRunes)
		second, secondErr := ChunkText(string(data), maxRunes, overlapRunes)
		if (firstErr == nil) != (secondErr == nil) ||
			(firstErr != nil && firstErr.Error() != secondErr.Error()) {
			t.Fatalf("ChunkText is nondeterministic: first=%v second=%v", firstErr, secondErr)
		}
		if firstErr != nil {
			return
		}
		if len(first) == 0 || len(first) > MaxTextChunks || len(first) != len(second) {
			t.Fatalf("invalid deterministic chunk count: first=%d second=%d", len(first), len(second))
		}
		for index, chunk := range first {
			if chunk != second[index] {
				t.Fatalf("chunk %d changed between identical calls", index)
			}
			if chunk.Ordinal != index || !utf8.ValidString(chunk.Text) ||
				strings.TrimSpace(chunk.Text) != chunk.Text ||
				utf8.RuneCountInString(chunk.Text) > maxRunes {
				t.Fatalf("chunk %d violates ordinal/text/rune bounds", index)
			}
			digest := sha256.Sum256([]byte(chunk.Text))
			if chunk.Digest != hex.EncodeToString(digest[:]) {
				t.Fatalf("chunk %d digest mismatch", index)
			}
		}
	})
}

func FuzzReadTextCanonicalAndBounded(f *testing.F) {
	for _, seed := range []struct {
		data     []byte
		maxBytes uint16
	}{
		{data: []byte("\xef\xbb\xbf\xe6\xa0\x87\xe9\xa2\x98\r\nfirst\rsecond\n"), maxBytes: 64},
		{data: []byte{0xff, 0xfe, 0xfd}, maxBytes: 3},
		{data: []byte("before\x00after"), maxBytes: 32},
		{data: []byte(" \r\n\t "), maxBytes: 8},
		{data: []byte("exact"), maxBytes: 4},
		{data: []byte("over-limit"), maxBytes: 3},
	} {
		f.Add(seed.data, seed.maxBytes)
	}

	f.Fuzz(func(t *testing.T, data []byte, maxSeed uint16) {
		// The public 4 MiB limit is covered by bounded DOC-001 qualification.
		// Keep mutation iterations small while still exercising every ReadText
		// validation and normalization branch.
		if len(data) > 64<<10 {
			t.Skip()
		}
		maxBytes := int64(maxSeed) + 1
		want, wantErr := canonicalReadTextResult(data, maxBytes)

		first, firstErr := ReadText(context.Background(), bytes.NewReader(data), maxBytes)
		second, secondErr := ReadText(context.Background(), bytes.NewReader(data), maxBytes)
		if wantErr != nil {
			if !errors.Is(firstErr, wantErr) || !errors.Is(secondErr, wantErr) || first != "" || second != "" {
				t.Fatalf("ReadText result = %q/%v then %q/%v; want empty/%v", first, firstErr, second, secondErr, wantErr)
			}
			return
		}
		if firstErr != nil || secondErr != nil {
			t.Fatalf("ReadText error = %v then %v; want success", firstErr, secondErr)
		}
		if first != want || second != want {
			t.Fatalf("ReadText result = %q then %q; want %q", first, second, want)
		}
		if len(first) > int(maxBytes) || !utf8.ValidString(first) || strings.ContainsAny(first, "\r\x00") || strings.TrimSpace(first) == "" {
			t.Fatalf("ReadText success violates byte/UTF-8/canonical-text bounds: %q", first)
		}
	})
}

func canonicalReadTextResult(data []byte, maxBytes int64) (string, error) {
	if int64(len(data)) > maxBytes {
		return "", ErrSourceTooLarge
	}
	if bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
		data = data[3:]
	}
	if !utf8.Valid(data) {
		return "", ErrInvalidUTF8
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return "", ErrBinaryText
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if strings.TrimSpace(text) == "" {
		return "", ErrEmptyText
	}
	return text, nil
}

func TestDetectTextFormat(t *testing.T) {
	t.Parallel()
	tests := map[string]TextFormat{
		"notes.TXT":      FormatText,
		"readme.md":      FormatMarkdown,
		"draft.markdown": FormatMarkdown,
		"manual.PDF":     FormatPDF,
	}
	for name, want := range tests {
		name, want := name, want
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := DetectTextFormat(name)
			if err != nil || got != want {
				t.Fatalf("DetectTextFormat(%q) = %q, %v; want %q", name, got, err, want)
			}
		})
	}
	if _, err := DetectTextFormat("archive.zip"); !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("unsupported error = %v; want ErrUnsupportedFormat", err)
	}
}

func TestReadTextNormalizesBOMAndLineEndings(t *testing.T) {
	t.Parallel()
	got, err := ReadText(context.Background(), strings.NewReader("\ufeff标题\r\n第一行\r第二行\n"), 128)
	if err != nil {
		t.Fatal(err)
	}
	if want := "标题\n第一行\n第二行\n"; got != want {
		t.Fatalf("text = %q; want %q", got, want)
	}
}

func TestReadTextRejectsInvalidInputs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data string
		max  int64
		want error
	}{
		{name: "too large", data: "12345", max: 4, want: ErrSourceTooLarge},
		{name: "invalid UTF-8", data: string([]byte{0xff}), max: 1, want: ErrInvalidUTF8},
		{name: "NUL", data: "a\x00b", max: 3, want: ErrBinaryText},
		{name: "empty", data: " \n\t", max: 3, want: ErrEmptyText},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := ReadText(context.Background(), strings.NewReader(test.data), test.max)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v; want %v", err, test.want)
			}
		})
	}
}

func TestReadTextAcceptsExactLimit(t *testing.T) {
	t.Parallel()
	got, err := ReadText(context.Background(), strings.NewReader("知识库"), int64(len("知识库")))
	if err != nil || got != "知识库" {
		t.Fatalf("ReadText = %q, %v", got, err)
	}
}

func TestReadTextStopsReaderWithoutProgress(t *testing.T) {
	t.Parallel()
	_, err := ReadText(context.Background(), zeroReader{}, 128)
	if !errors.Is(err, io.ErrNoProgress) {
		t.Fatalf("error = %v; want io.ErrNoProgress", err)
	}
}

func TestReadTextRejectsUnboundedLimit(t *testing.T) {
	t.Parallel()
	_, err := ReadText(context.Background(), strings.NewReader("text"), MaxTextSourceBytes+1)
	if err == nil {
		t.Fatal("ReadText accepted a limit above MaxTextSourceBytes")
	}
}

func TestChunkTextIsRuneSafeBoundedAndDeterministic(t *testing.T) {
	t.Parallel()
	text := "第一段讲本地知识库。\n第二段包含 MindWeaver 搜索能力。\n第三段用于验证稳定分块。"
	first, err := ChunkText(text, 18, 4)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ChunkText(text, 18, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) < 2 || len(first) != len(second) {
		t.Fatalf("unexpected chunk counts: %d and %d", len(first), len(second))
	}
	for index := range first {
		got := first[index]
		if got != second[index] {
			t.Fatalf("chunk %d is not deterministic: %#v != %#v", index, got, second[index])
		}
		if got.Ordinal != index {
			t.Fatalf("chunk %d ordinal = %d", index, got.Ordinal)
		}
		if utf8.RuneCountInString(got.Text) > 18 {
			t.Fatalf("chunk %d has %d runes: %q", index, utf8.RuneCountInString(got.Text), got.Text)
		}
		if len(got.Digest) != 64 {
			t.Fatalf("chunk %d digest length = %d", index, len(got.Digest))
		}
	}
}

func TestChunkTextHardBoundaryMakesProgress(t *testing.T) {
	t.Parallel()
	chunks, err := ChunkText("abcdefghij", 4, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 3 {
		t.Fatalf("chunk count = %d; want 3", len(chunks))
	}
	if chunks[0].Text != "abcd" || chunks[len(chunks)-1].Text != "ghij" {
		t.Fatalf("unexpected boundary chunks: %#v", chunks)
	}
}

func TestChunkTextValidation(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		max, overlap int
		want         error
	}{
		{max: 0, overlap: 0},
		{max: 4, overlap: -1},
		{max: 4, overlap: 2},
		{max: MaxChunkRunes + 1, overlap: 0},
	} {
		if _, err := ChunkText("content", test.max, test.overlap); err == nil {
			t.Fatalf("ChunkText(%d, %d) unexpectedly succeeded", test.max, test.overlap)
		}
	}
	if _, err := ChunkText(" \n", 4, 0); !errors.Is(err, ErrEmptyText) {
		t.Fatalf("empty error = %v; want ErrEmptyText", err)
	}
}

func TestChunkTextRejectsAllocationAmplification(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("a", MaxTextChunks+1)
	if _, err := ChunkText(text, 1, 0); !errors.Is(err, ErrChunkLimit) {
		t.Fatalf("error = %v; want ErrChunkLimit", err)
	}
}

func TestChunkTextRejectsBypassedReaderLimits(t *testing.T) {
	t.Parallel()
	if _, err := ChunkText(string([]byte{0xff}), 32, 0); !errors.Is(err, ErrInvalidUTF8) {
		t.Fatalf("invalid UTF-8 error = %v; want ErrInvalidUTF8", err)
	}
	tooLarge := strings.Repeat("a", int(MaxTextSourceBytes)+1)
	if _, err := ChunkText(tooLarge, MaxChunkRunes, 0); !errors.Is(err, ErrSourceTooLarge) {
		t.Fatalf("oversize error = %v; want ErrSourceTooLarge", err)
	}
}

func TestPublicTextByteCeilingFitsDefaultChunkBudgetForASCIIAndMultibyte(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{name: "maximum ASCII", text: strings.Repeat("a", int(MaxTextSourceBytes))},
		{name: "maximum four-byte UTF-8", text: strings.Repeat("😀", int(MaxTextSourceBytes)/len("😀"))},
	}
	minimumAdvance := DefaultChunkRunes/2 + 1 - DefaultChunkOverlap
	maximumWorstCase := 1 + (int(MaxTextSourceBytes)-DefaultChunkRunes+minimumAdvance-1)/minimumAdvance
	if maximumWorstCase > MaxTextChunks {
		t.Fatalf("public limit proof requires %d chunks; maximum is %d", maximumWorstCase, MaxTextChunks)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if int64(len(test.text)) != MaxTextSourceBytes {
				t.Fatalf("fixture bytes = %d, want %d", len(test.text), MaxTextSourceBytes)
			}
			admitted, err := ReadText(t.Context(), bytes.NewReader([]byte(test.text)), MaxTextSourceBytes)
			if err != nil || admitted != test.text {
				t.Fatalf("read maximum admitted source: bytes=%d, err=%v", len(admitted), err)
			}
			chunks, err := ChunkText(admitted, DefaultChunkRunes, DefaultChunkOverlap)
			if err != nil {
				t.Fatalf("chunk maximum admitted source: %v", err)
			}
			if len(chunks) > MaxTextChunks {
				t.Fatalf("chunks = %d, maximum = %d", len(chunks), MaxTextChunks)
			}
			for index, chunk := range chunks {
				if utf8.RuneCountInString(chunk.Text) > DefaultChunkRunes || len(chunk.Text) > 4*DefaultChunkRunes {
					t.Fatalf("chunk %d exceeds rune/byte proof: %d/%d", index, utf8.RuneCountInString(chunk.Text), len(chunk.Text))
				}
			}
		})
	}
}

type zeroReader struct{}

func (zeroReader) Read([]byte) (int, error) { return 0, nil }
