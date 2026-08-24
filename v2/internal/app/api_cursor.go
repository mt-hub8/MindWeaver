package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"
	"unicode/utf8"
)

const (
	apiCursorVersion              = 1
	maxAPICursorBytes             = 2048
	maxCursorJSONBytes            = 1024
	documentCursorKind            = "documents"
	collectionCursorKind          = "collections"
	memberCursorKind              = "collection-members"
	purgeCursorKind               = "document-purges"
	conversationCursorKind        = "conversations"
	conversationMessageCursorKind = "conversation-messages"
)

type apiCursor struct {
	Version int    `json:"version"`
	Kind    string `json:"kind"`
	Scope   string `json:"scope,omitempty"`
	Stamp   string `json:"stamp"`
	ID      string `json:"id"`
}

func encodeAPICursor(kind, scope string, stamp time.Time, id string) (string, error) {
	micros := stamp.UTC().UnixMicro()
	if kind == "" || micros < 0 || !stamp.Equal(time.UnixMicro(micros).UTC()) || !validIdentifier(id) {
		return "", errors.New("app: invalid cursor projection")
	}
	body, err := json.Marshal(apiCursor{
		Version: apiCursorVersion, Kind: kind, Scope: scope,
		Stamp: strconv.FormatInt(micros, 10), ID: id,
	})
	if err != nil {
		return "", fmt.Errorf("app: encode cursor: %w", err)
	}
	if len(body) > maxCursorJSONBytes {
		return "", errors.New("app: cursor projection exceeds limit")
	}
	return base64.RawURLEncoding.EncodeToString(body), nil
}

func decodeAPICursor(raw, wantKind, wantScope string) (time.Time, string, error) {
	if raw == "" || len(raw) > maxAPICursorBytes || !utf8.ValidString(raw) {
		return time.Time{}, "", errors.New("app: invalid cursor encoding")
	}
	body, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	if err != nil || len(body) == 0 || len(body) > maxCursorJSONBytes || !utf8.Valid(body) {
		return time.Time{}, "", errors.New("app: invalid cursor encoding")
	}
	if err := rejectDuplicateJSONKeys(body); err != nil {
		return time.Time{}, "", errors.New("app: invalid cursor JSON")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil || len(fields) < 4 || len(fields) > 5 {
		return time.Time{}, "", errors.New("app: invalid cursor JSON fields")
	}
	for _, required := range []string{"version", "kind", "stamp", "id"} {
		if _, ok := fields[required]; !ok {
			return time.Time{}, "", errors.New("app: cursor is missing a required field")
		}
	}
	for name := range fields {
		switch name {
		case "version", "kind", "scope", "stamp", "id":
		default:
			return time.Time{}, "", errors.New("app: cursor contains an unknown field")
		}
	}
	for _, name := range []string{"kind", "scope", "stamp", "id"} {
		rawValue, exists := fields[name]
		if exists {
			rawValue = bytes.TrimSpace(rawValue)
			if len(rawValue) < 2 || rawValue[0] != '"' {
				return time.Time{}, "", errors.New("app: cursor text field has the wrong JSON type")
			}
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var cursor apiCursor
	if err := decoder.Decode(&cursor); err != nil {
		return time.Time{}, "", errors.New("app: invalid cursor JSON")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return time.Time{}, "", errors.New("app: cursor has trailing data")
	}
	if cursor.Version != apiCursorVersion || cursor.Kind != wantKind || cursor.Scope != wantScope || !validIdentifier(cursor.ID) {
		return time.Time{}, "", errors.New("app: cursor does not match this catalog")
	}
	micros, err := strconv.ParseInt(cursor.Stamp, 10, 64)
	if err != nil || micros < 0 || strconv.FormatInt(micros, 10) != cursor.Stamp {
		return time.Time{}, "", errors.New("app: invalid cursor timestamp")
	}
	return time.UnixMicro(micros).UTC(), cursor.ID, nil
}
