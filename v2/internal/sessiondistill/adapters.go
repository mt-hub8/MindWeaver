package sessiondistill

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

func RequestFromNote(reader io.Reader) (Request, error) {
	if reader == nil {
		return Request{}, invalid()
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxTurnBytes+1))
	if err != nil {
		return Request{}, invalid()
	}
	if len(data) == 0 || len(data) > maxTurnBytes {
		return Request{}, newError(CodeInputLimitExceeded, fmt.Errorf("note size"))
	}
	return NewContentAddressedRequest([]InputTurn{{Role: RoleUser, Text: string(data)}})
}

func DecodeChatJSONL(reader io.Reader) (Request, error) {
	if reader == nil {
		return Request{}, invalid()
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxInputBytes+1))
	if err != nil {
		return Request{}, invalid()
	}
	if len(data) == 0 || len(data) > maxInputBytes || bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
		return Request{}, newError(CodeInputLimitExceeded, fmt.Errorf("chat input size"))
	}
	if !utf8.Valid(data) {
		return Request{}, invalid()
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4096), maxTurnBytes+1)
	turns := make([]InputTurn, 0)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 || len(line) > maxTurnBytes {
			return Request{}, invalid()
		}
		if err := validateJSONUnicodeEscapes(line); err != nil {
			return Request{}, invalid()
		}
		if err := scanStrictJSON(line); err != nil {
			return Request{}, invalid()
		}
		var shape map[string]json.RawMessage
		if err := json.Unmarshal(line, &shape); err != nil || exactKeys(shape, "role", "text") != nil {
			return Request{}, invalid()
		}
		var turn struct {
			Role Role   `json:"role"`
			Text string `json:"text"`
		}
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&turn); err != nil {
			return Request{}, invalid()
		}
		turns = append(turns, InputTurn{Role: turn.Role, Text: turn.Text})
		if len(turns) > maxTurns {
			return Request{}, newError(CodeInputLimitExceeded, fmt.Errorf("turn limit"))
		}
	}
	if err := scanner.Err(); err != nil {
		return Request{}, newError(CodeInputLimitExceeded, err)
	}
	return NewContentAddressedRequest(turns)
}

func NewContentAddressedRequest(input []InputTurn) (Request, error) {
	if len(input) == 0 || len(input) > maxTurns {
		return Request{}, invalid()
	}
	turns := make([]VisibleTurn, 0, len(input))
	eventIDs := make([][]byte, 0, len(input))
	total := 0
	for index, inputTurn := range input {
		if inputTurn.Role != RoleUser && inputTurn.Role != RoleAssistant || !utf8.ValidString(inputTurn.Text) || len(inputTurn.Text) == 0 || len(inputTurn.Text) > maxTurnBytes || hasForbiddenControl(inputTurn.Text) {
			return Request{}, invalid()
		}
		total += len(inputTurn.Text)
		if total > maxInputBytes {
			return Request{}, newError(CodeInputLimitExceeded, fmt.Errorf("input size"))
		}
		normalized := strings.ReplaceAll(strings.ReplaceAll(inputTurn.Text, "\r\n", "\n"), "\r", "\n")
		safeText, _, _ := redact(normalized)
		if len(safeText) == 0 || len(safeText) > maxTurnBytes {
			return Request{}, newError(CodeInputLimitExceeded, fmt.Errorf("safe text size"))
		}
		ordinal := uint32(index + 1)
		eventID := digest(
			"mindweaver/sessiondistill/adapter-event/v1",
			[]byte(fmt.Sprintf("%d", ordinal)),
			[]byte(inputTurn.Role),
			[]byte(safeText),
		)
		turns = append(turns, VisibleTurn{EventID: eventID, Ordinal: ordinal, Role: inputTurn.Role, Text: safeText})
		eventIDs = append(eventIDs, []byte(eventID))
	}
	sessionID := digest("mindweaver/sessiondistill/adapter-session/v1", eventIDs...)
	return Request{SchemaVersion: SchemaVersion, SessionID: sessionID, Turns: turns}, nil
}

func exactJSONLine(reader io.Reader, destination any) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return fmt.Errorf("JSON trailing data")
	}
	return nil
}
