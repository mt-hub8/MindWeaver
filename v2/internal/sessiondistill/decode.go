package sessiondistill

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"
)

const (
	maxInputBytes    = 1 << 20
	maxJSONDepth     = 32
	maxJSONTokens    = 20000
	maxTurns         = 1000
	maxTurnBytes     = 256 << 10
	maxOrdinal       = 20000
	maxSlices        = 4096
	maxSliceBytes    = 4096
	maxRenderedBytes = 1 << 20
)

func DecodeRequest(reader io.Reader) (Request, error) {
	if reader == nil {
		return Request{}, invalid()
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxInputBytes+1))
	if err != nil {
		return Request{}, invalid()
	}
	if len(data) == 0 || len(data) > maxInputBytes {
		return Request{}, newError(CodeInputLimitExceeded, fmt.Errorf("input size"))
	}
	if !utf8.Valid(data) || bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
		return Request{}, invalid()
	}
	if err := validateJSONUnicodeEscapes(data); err != nil {
		return Request{}, invalid()
	}
	if err := scanStrictJSON(data); err != nil {
		return Request{}, invalid()
	}
	if err := validateExactShape(data); err != nil {
		return Request{}, invalid()
	}
	var request Request
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return Request{}, invalid()
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return Request{}, invalid()
	}
	return request, nil
}

func validateJSONUnicodeEscapes(data []byte) error {
	inString := false
	for index := 0; index < len(data); index++ {
		switch data[index] {
		case '"':
			inString = !inString
		case '\\':
			if !inString || index+1 >= len(data) {
				continue
			}
			if data[index+1] != 'u' {
				index++
				continue
			}
			value, ok := parseHexCodeUnit(data, index+2)
			if !ok {
				return fmt.Errorf("invalid JSON unicode escape")
			}
			if value >= 0xdc00 && value <= 0xdfff {
				return fmt.Errorf("isolated JSON low surrogate")
			}
			if value >= 0xd800 && value <= 0xdbff {
				if index+12 > len(data) || data[index+6] != '\\' || data[index+7] != 'u' {
					return fmt.Errorf("isolated JSON high surrogate")
				}
				low, ok := parseHexCodeUnit(data, index+8)
				if !ok || low < 0xdc00 || low > 0xdfff {
					return fmt.Errorf("invalid JSON surrogate pair")
				}
				index += 11
				continue
			}
			index += 5
		}
	}
	return nil
}

func parseHexCodeUnit(data []byte, start int) (uint16, bool) {
	if start < 0 || start+4 > len(data) {
		return 0, false
	}
	var result uint16
	for _, value := range data[start : start+4] {
		result <<= 4
		switch {
		case value >= '0' && value <= '9':
			result += uint16(value - '0')
		case value >= 'a' && value <= 'f':
			result += uint16(value-'a') + 10
		case value >= 'A' && value <= 'F':
			result += uint16(value-'A') + 10
		default:
			return 0, false
		}
	}
	return result, true
}

func validateExactShape(data []byte) error {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return err
	}
	if err := exactKeys(top, "schema_version", "session_id", "turns"); err != nil {
		return err
	}
	var turns []json.RawMessage
	if err := json.Unmarshal(top["turns"], &turns); err != nil {
		return err
	}
	for _, raw := range turns {
		var turn map[string]json.RawMessage
		if err := json.Unmarshal(raw, &turn); err != nil {
			return err
		}
		if err := exactKeys(turn, "event_id", "ordinal", "role", "text"); err != nil {
			return err
		}
	}
	return nil
}

func exactKeys(object map[string]json.RawMessage, expected ...string) error {
	if len(object) != len(expected) {
		return fmt.Errorf("unexpected JSON shape")
	}
	for _, name := range expected {
		if _, ok := object[name]; !ok {
			return fmt.Errorf("unexpected JSON shape")
		}
	}
	return nil
}

type scanBudget struct{ tokens int }

func (budget *scanBudget) next(decoder *json.Decoder) (json.Token, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	budget.tokens++
	if budget.tokens > maxJSONTokens {
		return nil, fmt.Errorf("JSON token limit")
	}
	return token, nil
}

func scanStrictJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	budget := &scanBudget{}
	if err := scanValue(decoder, budget, 0); err != nil {
		return err
	}
	if _, err := budget.next(decoder); !errors.Is(err, io.EOF) {
		return fmt.Errorf("JSON trailing data")
	}
	return nil
}

func scanValue(decoder *json.Decoder, budget *scanBudget, depth int) error {
	token, err := budget.next(decoder)
	if err != nil {
		return err
	}
	delimiter, composite := token.(json.Delim)
	if !composite {
		return nil
	}
	if depth >= maxJSONDepth {
		return fmt.Errorf("JSON depth limit")
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := budget.next(decoder)
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("JSON key type")
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("duplicate JSON key")
			}
			seen[key] = struct{}{}
			if err := scanValue(decoder, budget, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := scanValue(decoder, budget, depth+1); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("JSON delimiter")
	}
	end, err := budget.next(decoder)
	if err != nil {
		return err
	}
	if delimiter == '{' && end != json.Delim('}') || delimiter == '[' && end != json.Delim(']') {
		return fmt.Errorf("JSON delimiter")
	}
	return nil
}
