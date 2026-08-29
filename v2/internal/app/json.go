package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
)

const (
	maxJSONNestingDepth = 64
	maxJSONTokens       = 4096
)

func decodeStrictJSON(request *http.Request, destination any) error {
	if destination == nil || !exactMediaType(request.Header, "application/json") || request.Header.Get("Content-Encoding") != "" {
		return errors.New("invalid JSON request metadata")
	}
	data, err := io.ReadAll(io.LimitReader(request.Body, maxJSONBodyBytes+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > maxJSONBodyBytes {
		return errors.New("JSON request exceeds limit")
	}
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return err
	}
	if err := rejectMisCasedKnownJSONFields(data, destination); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("JSON request has trailing data")
	}
	return nil
}

func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	budget := &jsonScanBudget{}
	if err := scanJSON(decoder, 0, budget); err != nil {
		return err
	}
	if _, err := budget.nextToken(decoder); !errors.Is(err, io.EOF) {
		return errors.New("JSON has trailing data")
	}
	return nil
}

type jsonScanBudget struct {
	tokens int
}

func (budget *jsonScanBudget) nextToken(decoder *json.Decoder) (json.Token, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	budget.tokens++
	if budget.tokens > maxJSONTokens {
		return nil, errors.New("JSON request exceeds token limit")
	}
	return token, nil
}

func scanJSON(decoder *json.Decoder, depth int, budget *jsonScanBudget) error {
	token, err := budget.nextToken(decoder)
	if err != nil {
		return err
	}
	delimiter, composite := token.(json.Delim)
	if !composite {
		return nil
	}
	if depth >= maxJSONNestingDepth {
		return errors.New("JSON request exceeds nesting limit")
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := budget.nextToken(decoder)
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("JSON key is not text")
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("duplicate JSON key %q", key)
			}
			seen[key] = struct{}{}
			if err := scanJSON(decoder, depth+1, budget); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := scanJSON(decoder, depth+1, budget); err != nil {
				return err
			}
		}
	default:
		return errors.New("invalid JSON delimiter")
	}
	end, err := budget.nextToken(decoder)
	if err != nil {
		return err
	}
	if (delimiter == '{' && end != json.Delim('}')) || (delimiter == '[' && end != json.Delim(']')) {
		return errors.New("mismatched JSON delimiter")
	}
	return nil
}

func rejectMisCasedKnownJSONFields(data []byte, destination any) error {
	typeOf := reflect.TypeOf(destination)
	if typeOf == nil {
		return errors.New("JSON destination is nil")
	}
	return validateExactJSONValue(data, typeOf)
}

func validateExactJSONValue(data []byte, typeOf reflect.Type) error {
	for typeOf.Kind() == reflect.Pointer {
		if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
			return nil
		}
		typeOf = typeOf.Elem()
	}
	switch typeOf.Kind() {
	case reflect.Struct:
		var object map[string]json.RawMessage
		if err := json.Unmarshal(data, &object); err != nil {
			return err
		}
		known := make(map[string]reflect.Type)
		for index := 0; index < typeOf.NumField(); index++ {
			field := typeOf.Field(index)
			if !field.IsExported() {
				continue
			}
			name := field.Name
			if tag, exists := field.Tag.Lookup("json"); exists {
				name = strings.Split(tag, ",")[0]
				if name == "-" {
					continue
				}
				if name == "" {
					name = field.Name
				}
			}
			known[name] = field.Type
		}
		for name, value := range object {
			fieldType, exact := known[name]
			if exact {
				if err := validateExactJSONValue(value, fieldType); err != nil {
					return err
				}
				continue
			}
			for knownName := range known {
				if strings.EqualFold(name, knownName) {
					return errors.New("JSON request contains a mis-cased known field")
				}
			}
		}
	case reflect.Slice, reflect.Array:
		var items []json.RawMessage
		if err := json.Unmarshal(data, &items); err != nil {
			return err
		}
		for _, item := range items {
			if err := validateExactJSONValue(item, typeOf.Elem()); err != nil {
				return err
			}
		}
	}
	return nil
}
