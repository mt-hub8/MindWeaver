package blob

import (
	"errors"
	"strings"
	"testing"
)

const (
	canonicalBlobIDPrefix          = "sha256:"
	canonicalBlobIDDigestHexLength = 64
)

func FuzzParseIDCanonical(f *testing.F) {
	for _, seed := range []string{
		canonicalBlobIDPrefix + strings.Repeat("0", canonicalBlobIDDigestHexLength),
		canonicalBlobIDPrefix + strings.Repeat("0123456789abcdef", canonicalBlobIDDigestHexLength/16),
		"SHA256:" + strings.Repeat("a", canonicalBlobIDDigestHexLength),
		canonicalBlobIDPrefix + strings.Repeat("A", canonicalBlobIDDigestHexLength),
		canonicalBlobIDPrefix + "../" + strings.Repeat("a", canonicalBlobIDDigestHexLength-3),
		"../objects/sha256",
		"",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 4<<10 {
			t.Skip()
		}
		wantValid := canonicalBlobID(raw)
		first, firstErr := ParseID(raw)
		second, secondErr := ParseID(raw)
		if !wantValid {
			if !errors.Is(firstErr, ErrInvalidID) || !errors.Is(secondErr, ErrInvalidID) || first != "" || second != "" {
				t.Fatalf("ParseID(%q) = %q/%v then %q/%v; want empty ErrInvalidID", raw, first, firstErr, second, secondErr)
			}
			return
		}
		if firstErr != nil || secondErr != nil {
			t.Fatalf("ParseID(%q) error = %v then %v", raw, firstErr, secondErr)
		}
		if first != second || first.String() != raw {
			t.Fatalf("ParseID(%q) normalized or changed the canonical ID: %q then %q", raw, first, second)
		}
	})
}

func canonicalBlobID(raw string) bool {
	if len(raw) != len(canonicalBlobIDPrefix)+canonicalBlobIDDigestHexLength || !strings.HasPrefix(raw, canonicalBlobIDPrefix) {
		return false
	}
	for _, character := range raw[len(canonicalBlobIDPrefix):] {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}
