package ideashook

import (
	"strings"
	"testing"
)

func TestDecodeOfficialRootEventsAndIgnoreSubagents(t *testing.T) {
	inputs := []struct {
		json     string
		event    string
		role     string
		content  string
		boundary string
		ignored  bool
	}{
		{json: `{"session_id":"s1","transcript_path":null,"cwd":"C:\\repo","hook_event_name":"SessionStart","model":"gpt","permission_mode":"default","source":"startup"}`, event: "SessionStart", boundary: "startup"},
		{json: `{"session_id":"s1","turn_id":"t1","transcript_path":null,"cwd":"C:\\repo","hook_event_name":"UserPromptSubmit","model":"gpt","permission_mode":"default","prompt":"用户内容"}`, event: "UserPromptSubmit", role: "user", content: "用户内容"},
		{json: `{"session_id":"s1","turn_id":"t1","transcript_path":null,"cwd":"C:\\repo","hook_event_name":"Stop","model":"gpt","permission_mode":"default","stop_hook_active":false,"last_assistant_message":"最终回答"}`, event: "Stop", role: "assistant", content: "最终回答"},
		{json: `{"session_id":"s1","transcript_path":null,"cwd":"C:\\repo","hook_event_name":"SessionEnd","reason":"other"}`, event: "SessionEnd", boundary: "other"},
		{json: `{"session_id":"s1","turn_id":"t1","agent_id":"a1","agent_type":"worker","transcript_path":null,"cwd":"C:\\repo","hook_event_name":"UserPromptSubmit","model":"gpt","permission_mode":"default","prompt":"internal"}`, ignored: true},
	}
	for index, test := range inputs {
		event, err := decodeHookInput(strings.NewReader(test.json))
		if err != nil || event.event != test.event || event.role != test.role || event.content != test.content || event.boundary != test.boundary || event.ignored != test.ignored {
			t.Fatalf("case %d event=%+v err=%v", index, event, err)
		}
	}
}

func TestDecodeRejectsUnknownDuplicateNullAgentAndInvalidUnicode(t *testing.T) {
	base := `{"session_id":"s1","turn_id":"t1","transcript_path":null,"cwd":"C:\\repo","hook_event_name":"UserPromptSubmit","model":"gpt","permission_mode":"default","prompt":"hello"}`
	invalid := []string{
		`{"session_id":"s1","turn_id":"t1","agent_id":null,"transcript_path":null,"cwd":"C:\\repo","hook_event_name":"UserPromptSubmit","model":"gpt","permission_mode":"default","prompt":"hello"}`,
		`{"session_id":"s1","turn_id":"t1","agent_type":null,"transcript_path":null,"cwd":"C:\\repo","hook_event_name":"UserPromptSubmit","model":"gpt","permission_mode":"default","prompt":"hello"}`,
		`{"session_id":"s1","session_id":"s2","turn_id":"t1","transcript_path":null,"cwd":"C:\\repo","hook_event_name":"UserPromptSubmit","model":"gpt","permission_mode":"default","prompt":"hello"}`,
		strings.Replace(base, `"prompt":"hello"`, `"Prompt":"hello"`, 1),
		strings.TrimSuffix(base, "}") + `,"tool_output":"secret"}`,
		strings.Replace(base, "hello", `\ud800`, 1),
		base + `{}`,
	}
	for index, input := range invalid {
		event, err := decodeHookInput(strings.NewReader(input))
		if err == nil || event != (captureEvent{}) {
			t.Fatalf("case %d event=%+v err=%v", index, event, err)
		}
	}
}

func TestDecodeIgnoresNullOrEmptyStopMessage(t *testing.T) {
	for _, message := range []string{"null", `""`} {
		input := `{"session_id":"s1","turn_id":"t1","transcript_path":null,"cwd":"C:\\repo","hook_event_name":"Stop","model":"gpt","permission_mode":"default","stop_hook_active":true,"last_assistant_message":` + message + `}`
		event, err := decodeHookInput(strings.NewReader(input))
		if err != nil || !event.ignored {
			t.Fatalf("message=%s event=%+v err=%v", message, event, err)
		}
	}
}
