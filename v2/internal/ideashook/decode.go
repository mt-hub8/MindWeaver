package ideashook

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxHookInputBytes = 512 << 10
	maxVisibleBytes   = 256 << 10
	maxMetadataBytes  = 32 << 10
	maxJSONDepth      = 8
	maxJSONTokens     = 128
)

type captureEvent struct {
	rawSession string
	rawTurn    string
	cwd        string
	event      string
	role       string
	content    string
	boundary   string
	ignored    bool
}

type sessionStartInput struct {
	SessionID      string  `json:"session_id"`
	TranscriptPath *string `json:"transcript_path"`
	CWD            string  `json:"cwd"`
	HookEventName  string  `json:"hook_event_name"`
	Model          string  `json:"model"`
	PermissionMode string  `json:"permission_mode"`
	Source         string  `json:"source"`
}

type sessionEndInput struct {
	SessionID      string  `json:"session_id"`
	TranscriptPath *string `json:"transcript_path"`
	CWD            string  `json:"cwd"`
	HookEventName  string  `json:"hook_event_name"`
	Reason         string  `json:"reason"`
}

type userPromptInput struct {
	SessionID      string  `json:"session_id"`
	TurnID         string  `json:"turn_id"`
	AgentID        *string `json:"agent_id,omitempty"`
	AgentType      *string `json:"agent_type,omitempty"`
	TranscriptPath *string `json:"transcript_path"`
	CWD            string  `json:"cwd"`
	HookEventName  string  `json:"hook_event_name"`
	Model          string  `json:"model"`
	PermissionMode string  `json:"permission_mode"`
	Prompt         string  `json:"prompt"`
}

type stopInput struct {
	SessionID            string  `json:"session_id"`
	TurnID               string  `json:"turn_id"`
	TranscriptPath       *string `json:"transcript_path"`
	CWD                  string  `json:"cwd"`
	HookEventName        string  `json:"hook_event_name"`
	Model                string  `json:"model"`
	PermissionMode       string  `json:"permission_mode"`
	StopHookActive       bool    `json:"stop_hook_active"`
	LastAssistantMessage *string `json:"last_assistant_message"`
}

func decodeHookInput(reader io.Reader) (captureEvent, error) {
	if reader == nil {
		return captureEvent{}, hookError(CodeInvalidInput, fmt.Errorf("hook input"))
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxHookInputBytes+1))
	if err != nil {
		return captureEvent{}, hookError(CodeInvalidInput, err)
	}
	if len(data) == 0 || len(data) > maxHookInputBytes {
		return captureEvent{}, hookError(CodeLimitExceeded, fmt.Errorf("hook input size"))
	}
	if !utf8.Valid(data) || bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
		return captureEvent{}, hookError(CodeInvalidInput, fmt.Errorf("hook input encoding"))
	}
	if err := validateJSONUnicodeEscapes(data); err != nil {
		return captureEvent{}, hookError(CodeInvalidInput, err)
	}
	if err := scanStrictJSON(data); err != nil {
		return captureEvent{}, hookError(CodeInvalidInput, err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return captureEvent{}, hookError(CodeInvalidInput, err)
	}
	rawEvent, exists := object["hook_event_name"]
	if !exists {
		return captureEvent{}, hookError(CodeInvalidInput, fmt.Errorf("hook event"))
	}
	var eventName string
	if err := json.Unmarshal(rawEvent, &eventName); err != nil {
		return captureEvent{}, hookError(CodeInvalidInput, err)
	}
	var event captureEvent
	switch eventName {
	case "SessionStart":
		if err := requireExactKeys(object, "session_id", "transcript_path", "cwd", "hook_event_name", "model", "permission_mode", "source"); err != nil {
			return captureEvent{}, hookError(CodeInvalidInput, err)
		}
		var input sessionStartInput
		if err := decodeExact(data, &input); err != nil || input.HookEventName != eventName || !validPermissionMode(input.PermissionMode) || !oneOf(input.Source, "startup", "resume", "clear", "compact") {
			return captureEvent{}, hookError(CodeInvalidInput, errors.Join(err, fmt.Errorf("session start")))
		}
		if !validCommon(input.SessionID, "", input.CWD, input.Model, input.TranscriptPath) {
			return captureEvent{}, hookError(CodeInvalidInput, fmt.Errorf("session start metadata"))
		}
		event = captureEvent{rawSession: input.SessionID, cwd: input.CWD, event: eventName, boundary: input.Source}
	case "SessionEnd":
		if err := requireExactKeys(object, "session_id", "transcript_path", "cwd", "hook_event_name", "reason"); err != nil {
			return captureEvent{}, hookError(CodeInvalidInput, err)
		}
		var input sessionEndInput
		if err := decodeExact(data, &input); err != nil || input.HookEventName != eventName || input.Reason != "other" {
			return captureEvent{}, hookError(CodeInvalidInput, errors.Join(err, fmt.Errorf("session end")))
		}
		if !validCommon(input.SessionID, "", input.CWD, "", input.TranscriptPath) {
			return captureEvent{}, hookError(CodeInvalidInput, fmt.Errorf("session end metadata"))
		}
		event = captureEvent{rawSession: input.SessionID, cwd: input.CWD, event: eventName, boundary: input.Reason}
	case "UserPromptSubmit":
		if err := requirePromptKeys(object); err != nil {
			return captureEvent{}, hookError(CodeInvalidInput, err)
		}
		agentFieldPresent, err := validateAgentFields(object)
		if err != nil {
			return captureEvent{}, hookError(CodeInvalidInput, err)
		}
		var input userPromptInput
		if err := decodeExact(data, &input); err != nil || input.HookEventName != eventName || !validPermissionMode(input.PermissionMode) {
			return captureEvent{}, hookError(CodeInvalidInput, errors.Join(err, fmt.Errorf("user prompt")))
		}
		if !validCommon(input.SessionID, input.TurnID, input.CWD, input.Model, input.TranscriptPath) || !validVisible(input.Prompt) || !validOptional(input.AgentID) || !validOptional(input.AgentType) {
			return captureEvent{}, hookError(CodeInvalidInput, fmt.Errorf("user prompt metadata"))
		}
		if agentFieldPresent {
			return captureEvent{ignored: true}, nil
		}
		event = captureEvent{rawSession: input.SessionID, rawTurn: input.TurnID, cwd: input.CWD, event: eventName, role: "user", content: normalizeLines(input.Prompt)}
	case "Stop":
		if err := requireExactKeys(object, "session_id", "turn_id", "transcript_path", "cwd", "hook_event_name", "model", "permission_mode", "stop_hook_active", "last_assistant_message"); err != nil {
			return captureEvent{}, hookError(CodeInvalidInput, err)
		}
		var input stopInput
		if err := decodeExact(data, &input); err != nil || input.HookEventName != eventName || !validPermissionMode(input.PermissionMode) {
			return captureEvent{}, hookError(CodeInvalidInput, errors.Join(err, fmt.Errorf("stop")))
		}
		if !validCommon(input.SessionID, input.TurnID, input.CWD, input.Model, input.TranscriptPath) || (input.LastAssistantMessage != nil && *input.LastAssistantMessage != "" && !validVisible(*input.LastAssistantMessage)) {
			return captureEvent{}, hookError(CodeInvalidInput, fmt.Errorf("stop metadata"))
		}
		if input.LastAssistantMessage == nil || *input.LastAssistantMessage == "" {
			return captureEvent{ignored: true}, nil
		}
		event = captureEvent{rawSession: input.SessionID, rawTurn: input.TurnID, cwd: input.CWD, event: eventName, role: "assistant", content: normalizeLines(*input.LastAssistantMessage)}
	default:
		return captureEvent{}, hookError(CodeInvalidInput, fmt.Errorf("unsupported hook event"))
	}
	return event, nil
}

func requirePromptKeys(object map[string]json.RawMessage) error {
	expected := []string{"session_id", "turn_id", "transcript_path", "cwd", "hook_event_name", "model", "permission_mode", "prompt"}
	if _, exists := object["agent_id"]; exists {
		expected = append(expected, "agent_id")
	}
	if _, exists := object["agent_type"]; exists {
		expected = append(expected, "agent_type")
	}
	return requireExactKeys(object, expected...)
}

func validateAgentFields(object map[string]json.RawMessage) (bool, error) {
	present := false
	for _, name := range []string{"agent_id", "agent_type"} {
		raw, exists := object[name]
		if !exists {
			continue
		}
		present = true
		var value string
		if err := json.Unmarshal(raw, &value); err != nil || !validMetadata(value, 1024) {
			return false, fmt.Errorf("hook agent metadata")
		}
	}
	return present, nil
}

func requireExactKeys(object map[string]json.RawMessage, expected ...string) error {
	if len(object) != len(expected) {
		return fmt.Errorf("hook input shape")
	}
	for _, key := range expected {
		if _, exists := object[key]; !exists {
			return fmt.Errorf("hook input shape")
		}
	}
	return nil
}

func decodeExact(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return fmt.Errorf("hook trailing data")
	}
	return nil
}

func validCommon(session, turn, cwd, model string, transcript *string) bool {
	if !validMetadata(session, 1024) || turn != "" && !validMetadata(turn, 1024) ||
		!validMetadata(cwd, maxMetadataBytes) || !filepath.IsAbs(cwd) ||
		model != "" && !validMetadata(model, 1024) {
		return false
	}
	return transcript == nil || validMetadata(*transcript, maxMetadataBytes)
}

func validOptional(value *string) bool {
	return value == nil || validMetadata(*value, 1024)
}

func validMetadata(value string, limit int) bool {
	return value != "" && len(value) <= limit && utf8.ValidString(value) && !hasForbiddenControl(value)
}

func validVisible(value string) bool {
	return value != "" && len(value) <= maxVisibleBytes && utf8.ValidString(value) && !hasForbiddenControl(value)
}

func hasForbiddenControl(value string) bool {
	for _, character := range value {
		if character == '\r' || character == '\n' || character == '\t' {
			continue
		}
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}

func validPermissionMode(value string) bool {
	return oneOf(value, "default", "acceptEdits", "plan", "dontAsk", "bypassPermissions")
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func normalizeLines(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n")
}

type jsonBudget struct{ tokens int }

func (budget *jsonBudget) next(decoder *json.Decoder) (json.Token, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	budget.tokens++
	if budget.tokens > maxJSONTokens {
		return nil, fmt.Errorf("hook JSON token limit")
	}
	return token, nil
}

func scanStrictJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	budget := &jsonBudget{}
	if err := scanJSONValue(decoder, budget, 0); err != nil {
		return err
	}
	if _, err := budget.next(decoder); !errors.Is(err, io.EOF) {
		return fmt.Errorf("hook JSON trailing data")
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder, budget *jsonBudget, depth int) error {
	token, err := budget.next(decoder)
	if err != nil {
		return err
	}
	delimiter, composite := token.(json.Delim)
	if !composite {
		return nil
	}
	if depth >= maxJSONDepth {
		return fmt.Errorf("hook JSON depth")
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
				return fmt.Errorf("hook JSON key")
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("hook JSON duplicate key")
			}
			seen[key] = struct{}{}
			if err := scanJSONValue(decoder, budget, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder, budget, depth+1); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("hook JSON delimiter")
	}
	end, err := budget.next(decoder)
	if err != nil {
		return err
	}
	if delimiter == '{' && end != json.Delim('}') || delimiter == '[' && end != json.Delim(']') {
		return fmt.Errorf("hook JSON delimiter")
	}
	return nil
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
			if !ok || value >= 0xdc00 && value <= 0xdfff {
				return fmt.Errorf("hook JSON unicode escape")
			}
			if value >= 0xd800 && value <= 0xdbff {
				if index+12 > len(data) || data[index+6] != '\\' || data[index+7] != 'u' {
					return fmt.Errorf("hook JSON surrogate")
				}
				low, ok := parseHexCodeUnit(data, index+8)
				if !ok || low < 0xdc00 || low > 0xdfff {
					return fmt.Errorf("hook JSON surrogate")
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
