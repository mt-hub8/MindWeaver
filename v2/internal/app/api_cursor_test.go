package app

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestAPICursorRoundTripAndScopeBinding(t *testing.T) {
	stamp := time.UnixMicro(1_765_432_109_876_543).UTC()
	raw, err := encodeAPICursor(memberCursorKind, "collection-one", stamp, "document-one")
	if err != nil {
		t.Fatal(err)
	}
	decodedStamp, decodedID, err := decodeAPICursor(raw, memberCursorKind, "collection-one")
	if err != nil || !decodedStamp.Equal(stamp) || decodedID != "document-one" {
		t.Fatalf("decode = %s, %q, %v", decodedStamp, decodedID, err)
	}
	if _, _, err := decodeAPICursor(raw, memberCursorKind, "collection-two"); err == nil {
		t.Fatal("cursor crossed collection scope")
	}
	if _, _, err := decodeAPICursor(raw, documentCursorKind, "collection-one"); err == nil {
		t.Fatal("cursor crossed catalog kind")
	}
}

func TestAPICursorRejectsNonCanonicalOrAmbiguousInput(t *testing.T) {
	encoded := func(body string) string {
		return base64.RawURLEncoding.EncodeToString([]byte(body))
	}
	tests := map[string]string{
		"unknown field":        encoded(`{"version":1,"kind":"documents","stamp":"1","id":"a","extra":true}`),
		"duplicate field":      encoded(`{"version":1,"kind":"documents","stamp":"1","id":"a","id":"b"}`),
		"case variant field":   encoded(`{"Version":1,"kind":"documents","stamp":"1","id":"a"}`),
		"case alias duplicate": encoded(`{"version":1,"Version":1,"kind":"documents","stamp":"1","id":"a"}`),
		"trailing JSON":        encoded(`{"version":1,"kind":"documents","stamp":"1","id":"a"}{}`),
		"wrong version":        encoded(`{"version":2,"kind":"documents","stamp":"1","id":"a"}`),
		"missing version":      encoded(`{"kind":"documents","stamp":"1","id":"a"}`),
		"leading zero stamp":   encoded(`{"version":1,"kind":"documents","stamp":"01","id":"a"}`),
		"signed stamp":         encoded(`{"version":1,"kind":"documents","stamp":"+1","id":"a"}`),
		"negative stamp":       encoded(`{"version":1,"kind":"documents","stamp":"-1","id":"a"}`),
		"numeric stamp":        encoded(`{"version":1,"kind":"documents","stamp":1,"id":"a"}`),
		"null scope":           encoded(`{"version":1,"kind":"documents","scope":null,"stamp":"1","id":"a"}`),
		"invalid identifier":   encoded("{\"version\":1,\"kind\":\"documents\",\"stamp\":\"1\",\"id\":\"bad\\u0000id\"}"),
		"non UTF-8 JSON":       base64.RawURLEncoding.EncodeToString([]byte{0xff, 0xfe}),
		"padded base64":        encoded(`{"version":1,"kind":"documents","stamp":"1","id":"a"}`) + "=",
		"invalid base64":       "not+a+url+cursor",
		"oversized transport":  strings.Repeat("a", maxAPICursorBytes+1),
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, _, err := decodeAPICursor(raw, documentCursorKind, ""); err == nil {
				t.Fatal("invalid cursor unexpectedly decoded")
			}
		})
	}

	invalidUTF8 := string([]byte{0xff})
	if _, _, err := decodeAPICursor(invalidUTF8, documentCursorKind, ""); err == nil {
		t.Fatal("invalid UTF-8 transport unexpectedly decoded")
	}
}

func TestAPICursorEncodeRequiresExactMicrosecondProjection(t *testing.T) {
	if _, err := encodeAPICursor(documentCursorKind, "", time.Unix(1, 123), "document"); err == nil {
		t.Fatal("sub-microsecond timestamp unexpectedly encoded")
	}
	if _, err := encodeAPICursor(documentCursorKind, "", time.Unix(-1, 0).UTC(), "document"); err == nil {
		t.Fatal("negative timestamp unexpectedly encoded")
	}
	if _, err := encodeAPICursor(documentCursorKind, "", time.UnixMicro(1).UTC(), " bad"); err == nil {
		t.Fatal("invalid identifier unexpectedly encoded")
	}
}
