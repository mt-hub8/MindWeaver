package sessiondistill

import (
	"bytes"
	"strings"
	"testing"
)

func TestDecodeRequestExactClosedShape(t *testing.T) {
	valid := `{"schema_version":1,"session_id":"` + testID('a') + `","turns":[{"event_id":"` + testID('b') + `","ordinal":1,"role":"user","text":"我的想法是先做 CLI。"}]}`
	request, err := DecodeRequest(strings.NewReader(valid))
	if err != nil || request.SessionID != testID('a') || len(request.Turns) != 1 {
		t.Fatalf("valid decode request=%+v err=%v", request, err)
	}
	invalid := []string{
		`{"schema_version":1,"schema_version":1,"session_id":"` + testID('a') + `","turns":[]}`,
		`{"Schema_Version":1,"session_id":"` + testID('a') + `","turns":[]}`,
		`{"schema_version":1,"session_id":"` + testID('a') + `","turns":[],"hidden_reasoning":"x"}`,
		`{"schema_version":1,"session_id":"` + testID('a') + `","turns":[{"event_id":"` + testID('b') + `","ordinal":1,"role":"user","text":"x","tool_output":"secret"}]}`,
		valid + ` {}`,
		`null`,
	}
	for index, input := range invalid {
		request, err := DecodeRequest(strings.NewReader(input))
		if err == nil || request.SchemaVersion != 0 || request.SessionID != "" || request.Turns != nil {
			t.Fatalf("case %d request=%+v err=%v", index, request, err)
		}
	}
}

func TestDecodeRequestEnforcesInputByteLimit(t *testing.T) {
	request, err := DecodeRequest(strings.NewReader(strings.Repeat("x", maxInputBytes+1)))
	if err == nil || CodeOf(err) != CodeInputLimitExceeded || request.SchemaVersion != 0 || request.SessionID != "" || request.Turns != nil {
		t.Fatalf("request=%+v err=%v code=%s", request, err, CodeOf(err))
	}
}

func TestDecodeRequestRejectsInvalidUTF8AndBOMBeforeJSONDecoding(t *testing.T) {
	valid := []byte(`{"schema_version":1,"session_id":"` + testID('a') + `","turns":[]}`)
	inputs := [][]byte{
		append([]byte{0xef, 0xbb, 0xbf}, valid...),
		append(append([]byte(nil), valid[:len(valid)-1]...), 0xff, '}'),
	}
	for index, input := range inputs {
		request, err := DecodeRequest(bytes.NewReader(input))
		if err == nil || request.SchemaVersion != 0 || request.SessionID != "" || request.Turns != nil {
			t.Fatalf("case %d request=%+v err=%v", index, request, err)
		}
	}
}

func TestDecodeRequestRejectsIsolatedSurrogatesAndAcceptsPairs(t *testing.T) {
	prefix := `{"schema_version":1,"session_id":"` + testID('a') + `","turns":[{"event_id":"` + testID('b') + `","ordinal":1,"role":"user","text":"`
	suffix := `"}]}`
	for _, escaped := range []string{`\ud800`, `\udc00`, `\ud800x`, `\ud800\u0041`} {
		request, err := DecodeRequest(strings.NewReader(prefix + escaped + suffix))
		if err == nil || request.SchemaVersion != 0 {
			t.Fatalf("isolated surrogate %q was accepted: %+v", escaped, request)
		}
	}
	request, err := DecodeRequest(strings.NewReader(prefix + `\ud83e\udde0` + suffix))
	if err != nil || request.Turns[0].Text != "🧠" {
		t.Fatalf("valid surrogate pair request=%+v err=%v", request, err)
	}
}
