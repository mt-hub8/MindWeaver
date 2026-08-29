package app

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeStrictJSONRequiresExactKnownFieldCaseAndRejectsUnknown(t *testing.T) {
	type nested struct {
		JobID string `json:"jobId"`
	}
	type input struct {
		DocumentID       string  `json:"documentId"`
		ExpectedRevision *int64  `json:"expectedRevision"`
		Nested           *nested `json:"nested,omitempty"`
	}
	tests := []struct {
		name string
		body string
		ok   bool
	}{
		{name: "exact", body: `{"documentId":"doc","expectedRevision":1,"nested":{"jobId":"job"}}`, ok: true},
		{name: "top level alias only", body: `{"DocumentId":"doc","expectedRevision":1}`},
		{name: "canonical and alias", body: `{"documentId":"doc","DocumentId":"other","expectedRevision":1}`},
		{name: "nested alias", body: `{"documentId":"doc","expectedRevision":1,"nested":{"JobId":"job"}}`},
		{name: "unknown", body: `{"documentId":"doc","expectedRevision":1,"extra":true}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest("POST", "/", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			var decoded input
			err := decodeStrictJSON(request, &decoded)
			if test.ok && err != nil {
				t.Fatalf("decode exact JSON: %v", err)
			}
			if !test.ok && err == nil {
				t.Fatal("unsafe JSON unexpectedly decoded")
			}
		})
	}
}

func TestStrictJSONScannerBoundsNestingAndAggregateTokens(t *testing.T) {
	deep := `{"value":` + strings.Repeat("[", maxJSONNestingDepth+1) + `0` + strings.Repeat("]", maxJSONNestingDepth+1) + `}`
	if err := rejectDuplicateJSONKeys([]byte(deep)); err == nil || !strings.Contains(err.Error(), "nesting limit") {
		t.Fatalf("deep JSON error = %v", err)
	}

	var amplified strings.Builder
	amplified.WriteString(`{"value":[`)
	for index := 0; index < maxJSONTokens; index++ {
		if index != 0 {
			amplified.WriteByte(',')
		}
		amplified.WriteByte('0')
	}
	amplified.WriteString(`]}`)
	if amplified.Len() > int(maxJSONBodyBytes) {
		t.Fatalf("token amplification is %d bytes; must remain below the body limit", amplified.Len())
	}
	if err := rejectDuplicateJSONKeys([]byte(amplified.String())); err == nil || !strings.Contains(err.Error(), "token limit") {
		t.Fatalf("amplified JSON error = %v", err)
	}
}
