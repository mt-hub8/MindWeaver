package platform

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"strings"

	"github.com/mt-hub8/MindWeaver/v2/platform/apperror"
)

// ID is a canonical RFC 4122 identifier. A distinct type prevents accidental
// interchange with unvalidated user input.
type ID string

func (id ID) String() string {
	return string(id)
}

func (id ID) IsZero() bool {
	return id == ""
}

// ParseID validates and normalizes a canonical UUID string.
func ParseID(value string) (ID, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return "", apperror.New(apperror.KindInvalid, "id.invalid", "identifier must be a canonical UUID")
	}
	compact := strings.ReplaceAll(value, "-", "")
	if len(compact) != 32 {
		return "", apperror.New(apperror.KindInvalid, "id.invalid", "identifier must be a canonical UUID")
	}
	if _, err := hex.DecodeString(compact); err != nil {
		return "", apperror.Wrap(err, apperror.KindInvalid, "id.invalid", "platform.parse_id", "identifier must be a canonical UUID")
	}
	return ID(value), nil
}

// IDGenerator produces identifiers. Entropy reads are I/O and therefore take
// a context even though crypto/rand itself cannot be interrupted mid-read.
type IDGenerator interface {
	New(context.Context) (ID, error)
}

// RandomIDGenerator generates RFC 4122 version 4 identifiers. Reader defaults
// to crypto/rand.Reader and is injectable for deterministic tests.
type RandomIDGenerator struct {
	Reader io.Reader
}

func (g RandomIDGenerator) New(ctx context.Context) (ID, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	reader := g.Reader
	if reader == nil {
		reader = rand.Reader
	}
	var raw [16]byte
	if _, err := io.ReadFull(reader, raw[:]); err != nil {
		return "", apperror.Wrap(err, apperror.KindUnavailable, "id.entropy_unavailable", "platform.random_id", "identifier could not be generated")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	encoded := make([]byte, 36)
	hex.Encode(encoded[0:8], raw[0:4])
	encoded[8] = '-'
	hex.Encode(encoded[9:13], raw[4:6])
	encoded[13] = '-'
	hex.Encode(encoded[14:18], raw[6:8])
	encoded[18] = '-'
	hex.Encode(encoded[19:23], raw[8:10])
	encoded[23] = '-'
	hex.Encode(encoded[24:36], raw[10:16])
	id, err := ParseID(string(encoded))
	if err != nil {
		return "", errors.New("platform: generated invalid identifier")
	}
	return id, nil
}
